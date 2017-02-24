package thecomposables

import (
	"fmt"
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

type page struct {
	Title  string
	Body   []byte
	ID     string
	Markup template.HTML
}

// Dashes in page IDs (slugs) are mapped to spaces in the title:
func titleToID(title string) string { return strings.Replace(title, " ", "-", -1) }
func idToTitle(id string) string    { return strings.Replace(id, "-", " ", -1) }

// Load implements the PropertyLoadSaver interface for *page.
// Body is parsed as Markdown, and Title is converted to ID.
func (p *page) Load(props []datastore.Property) error {
	for _, prop := range props {
		switch prop.Name {
		case "Title":
			title, ok := prop.Value.(string)
			if !ok {
				return fmt.Errorf("Title value [%v] is not a string", prop.Value)
			}
			p.Title = title
			p.ID = titleToID(title)

		case "Body":
			body, ok := prop.Value.([]byte)
			if !ok {
				return fmt.Errorf("Title value [%v] is not a []byte", prop.Value)
			}
			p.Body = body

			//content is trusted because editing is locked to admins.
			//github.com/microcosm-cc/bluemonday for more security.
			p.Markup = template.HTML(blackfriday.MarkdownCommon(body))
		}
	}
	return nil
}

// Save implements the PropertyLoadSaver interface for *page.
// Only Title and Body are saved, ID and Markup are generated
// in Load().
func (p *page) Save() ([]datastore.Property, error) {
	return []datastore.Property{
		{Name: "Title", Value: p.Title},
		{Name: "Body", Value: p.Body, NoIndex: true},
	}, nil
}

// pageIndex implements alphabetical sort by Title for []*page
type pageIndex []*page

func (a pageIndex) Len() int      { return len(a) }
func (a pageIndex) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a pageIndex) Less(i, j int) bool {
	return strings.ToLower(a[i].Title) < strings.ToLower(a[j].Title)
}

// TemplateData is info needed to render a edit or view page
type TemplateData struct {
	Page *page
	User *user.User
}

func (p *page) save(c context.Context) error {
	k := datastore.NewKey(c, "Page", p.Title, 0, nil)
	_, err := datastore.Put(c, k, p)
	return err
}

func loadPage(c context.Context, title string) (*page, error) {
	k := datastore.NewKey(c, "Page", title, 0, nil)
	var p page
	err := datastore.Get(c, k, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func viewHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(c, title)
	if err != nil {
		http.Redirect(w, r, "/edit/"+titleToID(title), http.StatusFound)
		return
	}
	renderTemplate(w, "view", TemplateData{
		Page: p,
		User: user.Current(c),
	})
}

func editHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	currentUser := user.Current(c)
	if currentUser == nil || !currentUser.Admin {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	p, err := loadPage(c, title)
	if err != nil {
		p = &page{
			Title: title,
			ID:    titleToID(title),
		}
	}

	renderTemplate(w, "edit", TemplateData{
		Page: p,
		User: user.Current(c),
	})
}

func saveHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	currentUser := user.Current(c)
	if currentUser == nil || !currentUser.Admin {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	body := r.FormValue("body")
	p := &page{
		Title: title,
		Body:  []byte(body),
	}
	err := p.save(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+titleToID(title), http.StatusFound)
}

func deleteHandler(c context.Context, w http.ResponseWriter, r *http.Request, title string) {
	currentUser := user.Current(c)
	if currentUser == nil || !currentUser.Admin {
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
		title := idToTitle(m[2])
		fn(c, w, r, title)
	}
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
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))
	http.HandleFunc("/delete/", makeHandler(deleteHandler))
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/", home)
}
