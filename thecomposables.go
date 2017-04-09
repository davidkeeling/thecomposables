package thecomposables

import (
	"html/template"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/net/context"

	"strings"

	"fmt"

	"github.com/russross/blackfriday"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/user"
)

// request is a container for session/request data
type request struct {
	c context.Context
	w http.ResponseWriter
	r *http.Request
	u *user.User
}

var templates = template.Must(template.ParseFiles("tpl/edit.html", "tpl/view.html", "tpl/history.html", "tpl/index.html", "tpl/components.html"))
var validPagePath = regexp.MustCompile("^/(edit|save|view|delete|history)/([a-zA-Z0-9',-]+)$")
var validUserPath = regexp.MustCompile("^/user/(login|logout)$")

// Dashes in page IDs (slugs) are mapped to spaces in the title:
func titleToID(title string) string { return strings.Replace(title, " ", "-", -1) }
func idToTitle(id string) string    { return strings.Replace(id, "-", " ", -1) }

type templateData struct {
	Page     *page
	User     *user.User
	Mode     renderMode
	Pages    []*page
	Redirect string
	IsAdmin  bool
	IsDev    bool
}

type renderMode string

const (
	edit    renderMode = "edit"
	view               = "view"
	history            = "history"
)

const numVersions = 10

func makePageHandler(fn func(*request, *page), adminOnly bool) http.HandlerFunc {
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

func render(r *request, p *page, mode renderMode) error {
	pages, err := getPages(r.c)
	if err != nil {
		return err
	}

	return templates.ExecuteTemplate(r.w, string(mode)+".html", templateData{
		Page:     p,
		User:     r.u,
		Mode:     mode,
		Redirect: fmt.Sprintf("/view/%s", p.ID),
		Pages:    pages,
		IsAdmin:  r.u != nil && r.u.Admin,
		IsDev:    appengine.IsDevAppServer(),
	})
}

func viewHandler(r *request, p *page) {
	p.Markup = getMarkup(p.Body)
	err := render(r, p, view)
	if err != nil {
		handleError(r, err)
	}
}

func historyHandler(r *request, p *page) {
	p.Markup = getMarkup(p.Body)
	for i := range p.Versions {
		p.Versions[i].Markup = getMarkup(p.Versions[i].Body)
	}
	err := render(r, p, history)
	if err != nil {
		handleError(r, err)
	}
}

func editHandler(r *request, p *page) {
	err := render(r, p, edit)
	if err != nil {
		handleError(r, err)
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
		handleError(r, err)
		return
	}
	if p.DoesNotExist {
		clearPageCache(r.c)
	}
	http.Redirect(r.w, r.r, "/view/"+p.ID, http.StatusFound)
}

func deleteHandler(r *request, p *page) {
	if p.DoesNotExist {
		handleError(r, fmt.Errorf("%s does not exist; cannot delete", p.Title))
		return
	}
	err := datastore.Delete(r.c, p.Key)
	if err != nil {
		handleError(r, err)
		return
	}
	clearPageCache(r.c)
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

func search(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	pageName := r.FormValue("pageName")
	page, err := loadPage(c, pageName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if page.DoesNotExist {
		if u == nil || !u.Admin {
			http.Error(w, fmt.Sprintf("No such page (%s)", pageName), http.StatusNotFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/edit/%s", page.ID), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/view/%s", page.ID), http.StatusFound)
}

func handleError(r *request, err error) {
	http.Error(r.w, err.Error(), http.StatusInternalServerError)
}

func getMarkup(body []byte) template.HTML {
	return template.HTML(blackfriday.MarkdownCommon(body))
}

func init() {
	http.HandleFunc("/view/", makePageHandler(viewHandler, false))
	http.HandleFunc("/edit/", makePageHandler(editHandler, true))
	http.HandleFunc("/history/", makePageHandler(historyHandler, true))
	http.HandleFunc("/save/", makePageHandler(saveHandler, true))
	http.HandleFunc("/delete/", makePageHandler(deleteHandler, true))
	http.HandleFunc("/user/", userHandler)
	http.HandleFunc("/search", search)
	http.HandleFunc("/", home)
}
