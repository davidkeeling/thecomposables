package thecomposables

import (
	"html/template"
	"net/http"
	"regexp"
	"sort"

	"golang.org/x/net/context"

	"strings"

	"github.com/russross/blackfriday"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"
)

// Page is a document in the wiki
type Page struct {
	Title  string
	Body   []byte
	ID     string        `datastore:"-"`
	Markup template.HTML `datastore:"-"`
}

// PageIndex implements alphabetical sort by Title for []Page
type PageIndex []Page

func (a PageIndex) Len() int           { return len(a) }
func (a PageIndex) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PageIndex) Less(i, j int) bool { return a[i].Title < a[j].Title }

// TemplateData is info needed to render a edit or view page
type TemplateData struct {
	Page *Page
	User *user.User
}

func (p *Page) save(c context.Context) error {
	k := datastore.NewKey(c, "Page", p.Title, 0, nil)
	_, err := datastore.Put(c, k, p)
	return err
}

func loadPage(c context.Context, title string) (*Page, error) {
	k := datastore.NewKey(c, "Page", title, 0, nil)
	var p Page
	err := datastore.Get(c, k, &p)
	if err != nil {
		return nil, err
	}
	p.Markup = template.HTML(blackfriday.MarkdownCommon(p.Body))
	p.ID = strings.Replace(p.Title, " ", "-", -1)
	return &p, nil
}

func viewHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(c, title)
	if err != nil {
		http.Redirect(w, r, "/edit/"+strings.Replace(title, " ", "-", -1), http.StatusFound)
		return
	}
	renderTemplate(w, "view", TemplateData{
		Page: p,
		User: user.Current(c),
	})
}

func editHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	if !user.Current(c).Admin {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	p, err := loadPage(c, title)
	if err != nil {
		p = &Page{
			Title: title,
			ID:    strings.Replace(title, " ", "-", -1),
		}
	}

	renderTemplate(w, "edit", TemplateData{
		Page: p,
		User: user.Current(c),
	})
}

func saveHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	if !user.Current(c).Admin {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	body := r.FormValue("body")
	p := &Page{
		Title: title,
		Body:  []byte(body),
	}
	err := p.save(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+strings.Replace(title, " ", "-", -1), http.StatusFound)
}

func deleteHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	if !user.Current(c).Admin {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	k := datastore.NewKey(c, "Page", title, 0, nil)
	err := datastore.Delete(c, k)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
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

var templates = template.Must(template.ParseFiles("tpl/edit.html", "tpl/view.html", "tpl/index.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, data TemplateData) {
	err := templates.ExecuteTemplate(w, tmpl+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var validPath = regexp.MustCompile("^/(edit|save|view|delete)/([a-zA-Z0-9',-]+)$")

func makeHandler(fn func(context.Context, http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		c := appengine.NewContext(r)
		title := strings.Replace(m[2], "-", " ", -1)
		fn(c, w, r, title)
	}
}

func home(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	t := datastore.NewQuery("Page").Run(c)
	pages := PageIndex{}
	var homepagecontent template.HTML
	for {
		var p Page
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
		p.ID = strings.Replace(p.Title, " ", "-", -1)
		pages = append(pages, p)
	}
	sort.Sort(pages)

	err := templates.ExecuteTemplate(w, "index.html", struct {
		Pages        PageIndex
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
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))
	http.HandleFunc("/delete/", makeHandler(deleteHandler))
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/", home)
}
