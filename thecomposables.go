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

	"encoding/json"

	"github.com/russross/blackfriday"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/user"
)

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
	if p.DoesNotExist {
		clearPageCache(r.c)
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

func home(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	pages, err := getPages(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var homepagecontent template.HTML
	for _, p := range pages {
		if p.Title == "Introduction" {
			homepagecontent = template.HTML(blackfriday.MarkdownCommon(p.Body))
		}
	}

	u := user.Current(c)
	err = templates.ExecuteTemplate(w, "index.html", struct {
		Pages        pageIndex
		Introduction template.HTML
		User         *user.User
		IsAdmin      bool
		Redirect     string
	}{
		pages,
		homepagecontent,
		u,
		u != nil && u.Admin,
		"/",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getPages(c context.Context) (pageIndex, error) {
	pages := pageIndex{}
	val, err := memcache.Get(c, "pages")
	if err == nil {
		jErr := json.Unmarshal(val.Value, &pages)
		if jErr == nil {
			return pages, nil
		}
		log.Errorf(c, "Unmarshalling pages from memcache: %s", jErr)
	}
	if err != memcache.ErrCacheMiss {
		log.Errorf(c, "Fetching pages from memcache: %s", err)
	}

	t := datastore.NewQuery("Page").Ancestor(pageParentKey(c)).Run(c)
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
		p.ID = titleToID(p.Title)
		pages = append(pages, &p)
	}
	sort.Sort(pages)

	pageJSON, err := json.Marshal(pages)
	if err == nil {
		mErr := memcache.Set(c, &memcache.Item{
			Key:   "pages",
			Value: pageJSON,
		})
		if mErr != nil {
			log.Errorf(c, "Setting pages in memcache: %s", mErr)
		}
	}

	return pages, nil
}

func clearPageCache(c context.Context) {
	log.Infof(c, "Clearing page cache")
	err := memcache.Delete(c, "pages")
	if err != nil {
		log.Errorf(c, "Resetting pages memcache: %s", err)
	}
}

func init() {
	http.HandleFunc("/view/", pageHandler(viewHandler, false))
	http.HandleFunc("/edit/", pageHandler(editHandler, true))
	http.HandleFunc("/history/", pageHandler(historyHandler, true))
	http.HandleFunc("/save/", pageHandler(saveHandler, true))
	http.HandleFunc("/delete/", pageHandler(deleteHandler, true))
	http.HandleFunc("/user/", userHandler)
	http.HandleFunc("/search", search)
	http.HandleFunc("/", home)
}
