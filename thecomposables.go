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

var templates = template.Must(template.ParseFiles("tpl/edit.html", "tpl/view.html", "tpl/history.html", "tpl/index.html", "tpl/components.html"))
var validPagePath = regexp.MustCompile("^/(edit|save|view|delete|history)/([a-zA-Z0-9',-]+)$")
var validUserPath = regexp.MustCompile("^/user/(login|logout)$")

// Dashes in page IDs (slugs) are mapped to spaces in the title:
func titleToID(title string) string { return strings.Replace(title, " ", "-", -1) }
func idToTitle(id string) string    { return strings.Replace(id, "-", " ", -1) }

type templateData struct {
	Page *page
	User *user.User
	Mode renderMode
}

// request is a container for session/request data
type request struct {
	c context.Context
	w http.ResponseWriter
	r *http.Request
	u *user.User
}

type renderMode string

const (
	edit    renderMode = "edit"
	view               = "view"
	history            = "history"
)

const numVersions = 10

func pageHandler(fn func(*request, *page), adminOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		matches := validPagePath.FindStringSubmatch(r.URL.Path)
		if matches == nil {
			http.NotFound(w, r)
			return
		}
		c := appengine.NewContext(r)

		u := user.Current(c)
		if adminOnly && (u == nil || !u.Admin) {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		title := matches[2]
		p, err := loadPage(c, idToTitle(title))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fn(&request{c, w, r, u}, p)
	}
}

func (r *request) handleError(err error) {
	http.Error(r.w, err.Error(), http.StatusInternalServerError)
}

func render(r *request, p *page, mode renderMode) error {
	return templates.ExecuteTemplate(r.w, string(mode)+".html", templateData{
		Page: p,
		User: r.u,
		Mode: mode,
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
	p.Markup = template.HTML(blackfriday.MarkdownCommon(p.Body))
	for i := range p.Versions {
		p.Versions[i].Markup = template.HTML(blackfriday.MarkdownCommon(p.Versions[i].Body))
	}
	err := render(r, p, history)
	if err != nil {
		r.handleError(err)
	}
}

func editHandler(r *request, p *page) {
	err := render(r, p, edit)
	if err != nil {
		r.handleError(err)
	}
}

func saveHandler(r *request, p *page) {
	p.Versions = append([]version{{Body: p.Body, Date: time.Now()}}, p.Versions...)
	if len(p.Versions) > numVersions {
		p.Versions = p.Versions[:numVersions]
	}
	p.Body = []byte(r.r.FormValue("body"))
	err := p.save(r.c)
	if err != nil {
		r.handleError(err)
		return
	}
	http.Redirect(r.w, r.r, "/view/"+p.ID, http.StatusFound)
}

func deleteHandler(r *request, p *page) {
	if p.DoesNotExist {
		r.handleError(fmt.Errorf("%s does not exist; cannot delete", p.Title))
		return
	}
	err := datastore.Delete(r.c, p.Key)
	if err != nil {
		r.handleError(err)
		return
	}
	http.Redirect(r.w, r.r, "/", http.StatusFound)
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	matches := validUserPath.FindStringSubmatch(r.URL.Path)
	if matches == nil {
		http.NotFound(w, r)
		return
	}
	redirect := r.FormValue("redirect")
	if redirect == "" {
		redirect = "/"
	}

	c := appengine.NewContext(r)
	var (
		dest string
		err  error
	)
	if matches[1] == "login" {
		dest, err = user.LoginURL(c, redirect)
	} else {
		dest, err = user.LogoutURL(c, redirect)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, dest, http.StatusFound)
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
		User         *user.User
	}{
		pages,
		homepagecontent,
		user.Current(c),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func init() {
	http.HandleFunc("/view/", pageHandler(viewHandler, false))
	http.HandleFunc("/edit/", pageHandler(editHandler, true))
	http.HandleFunc("/history/", pageHandler(historyHandler, true))
	http.HandleFunc("/save/", pageHandler(saveHandler, true))
	http.HandleFunc("/delete/", pageHandler(deleteHandler, true))
	http.HandleFunc("/user/", userHandler)
	http.HandleFunc("/", home)
}
