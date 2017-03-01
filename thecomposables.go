package thecomposables

import (
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"time"

	"golang.org/x/net/context"

	"strings"

	"fmt"

	"github.com/russross/blackfriday"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"
)

var templates = template.Must(template.ParseFiles("tpl/edit.html", "tpl/view.html", "tpl/history.html", "tpl/index.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|delete|history)/([a-zA-Z0-9',-]+)$")

// Dashes in page IDs (slugs) are mapped to spaces in the title:
func titleToID(title string) string { return strings.Replace(title, " ", "-", -1) }
func idToTitle(id string) string    { return strings.Replace(id, "-", " ", -1) }

type templateData struct {
	Page *page
	User *user.User
}

type request struct {
	c context.Context
	w http.ResponseWriter
	r *http.Request
}

func pageHandler(fn func(*request, *page)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		c := appengine.NewContext(r)
		title := m[2]
		p, err := loadPage(c, idToTitle(title))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.ID = title
		fn(&request{c, w, r}, p)
	}
}

func (r *request) handleError(err error) {
	http.Error(r.w, err.Error(), http.StatusInternalServerError)
}

func (r *request) handleUnauthorized() {
	http.Error(r.w, "", http.StatusUnauthorized)
}

func render(r *request, p *page, mode renderMode) error {
	return templates.ExecuteTemplate(r.w, string(mode)+".html", templateData{
		Page: p,
		User: user.Current(r.c),
	})
}

func viewHandler(r *request, p *page) {
	p.Markup = template.HTML(blackfriday.MarkdownCommon(p.Body))
	err := render(r, p, view)
	if err != nil {
		r.handleError(err)
	}
}

func historyHandler(r *request, p *page) {
	for _, v := range p.Versions {
		p.VersionMarkup = append(p.VersionMarkup, template.HTML(blackfriday.MarkdownCommon(v.Body)))
	}
	p.Markup = template.HTML(blackfriday.MarkdownCommon(p.Body))
	err := render(r, p, history)
	if err != nil {
		r.handleError(err)
	}
}

func editHandler(r *request, p *page) {
	currentUser := user.Current(r.c)
	if currentUser == nil || !currentUser.Admin {
		r.handleUnauthorized()
		return
	}

	err := render(r, p, edit)
	if err != nil {
		r.handleError(err)
	}
}

func saveHandler(r *request, p *page) {
	currentUser := user.Current(r.c)
	if currentUser == nil || !currentUser.Admin {
		r.handleUnauthorized()
		return
	}
	p.Versions = append([]version{{p.Body, time.Now()}}, p.Versions...)
	if len(p.Versions) > 10 {
		p.Versions = p.Versions[0:10]
	}
	p.Body = []byte(r.r.FormValue("body"))
	err := p.save(r.c)
	if err != nil {
		r.handleError(err)
		return
	}
	http.Redirect(r.w, r.r, "/view/"+titleToID(p.Title), http.StatusFound)
}

func deleteHandler(r *request, p *page) {
	if p.DoesNotExist {
		r.handleError(fmt.Errorf("%s does not exist; cannot delete", p.Title))
		return
	}
	currentUser := user.Current(r.c)
	if currentUser == nil || !currentUser.Admin {
		r.handleUnauthorized()
		return
	}
	k := datastore.NewKey(r.c, "Page", p.Title, 0, nil)
	err := datastore.Delete(r.c, k)
	if err != nil {
		r.handleError(err)
		return
	}
	http.Redirect(r.w, r.r, "/", http.StatusFound)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	loginURL, err := user.LoginURL(c, "/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func home(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	t := datastore.NewQuery("Page").Run(c)
	pages := pageIndex{}
	var homepagecontent template.HTML
	for {
		var p page
		_, err := t.Next(&p)
		if err == datastore.Done {
			break
		}
		if err != nil {
			log.Errorf(c, "Loading page: %s", err)
			break
		}
		if p.Title == "Introduction" {
			homepagecontent = template.HTML(blackfriday.MarkdownCommon(p.Body))
		}
		p.ID = titleToID(p.Title)
		pages = append(pages, &p)
	}
	sort.Sort(pages)

	err := templates.ExecuteTemplate(w, "index.html", struct {
		Pages        pageIndex
		Introduction template.HTML
	}{
		pages,
		homepagecontent,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func init() {
	http.HandleFunc("/view/", pageHandler(viewHandler))
	http.HandleFunc("/edit/", pageHandler(editHandler))
	http.HandleFunc("/history/", pageHandler(historyHandler))
	http.HandleFunc("/save/", pageHandler(saveHandler))
	http.HandleFunc("/delete/", pageHandler(deleteHandler))
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/", home)
}
