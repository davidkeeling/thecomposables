package thecomposables

import (
	"html/template"
	"strings"

	"golang.org/x/net/context"

	"time"

	"google.golang.org/appengine/datastore"
)

type renderMode string

const (
	edit    renderMode = "edit"
	view               = "view"
	history            = "history"
)

func titleToKey(c context.Context, title string) *datastore.Key {
	return datastore.NewKey(c, "Page", title, 0, nil)
}

type page struct {
	ID            string `datastore:"-"`
	Title         string
	Body          []byte        `datastore:",noindex"`
	Markup        template.HTML `datastore:"-"`
	Versions      []version
	VersionMarkup []template.HTML `datastore:"-"`
	DoesNotExist  bool            `datastore:"-"`
}

type version struct {
	Body []byte `datastore:",noindex"`
	Date time.Time
}

func (p *page) save(c context.Context) error {
	_, err := datastore.Put(c, titleToKey(c, p.Title), p)
	return err
}

func loadPage(c context.Context, title string) (*page, error) {
	var p page
	err := datastore.Get(c, titleToKey(c, title), &p)
	if err == datastore.ErrNoSuchEntity {
		p = page{
			Title:        title,
			ID:           titleToID(title),
			DoesNotExist: true,
		}
	} else if err != nil {
		return nil, err
	}

	return &p, nil
}

// pageIndex implements alphabetical sort by Title for []*page
type pageIndex []*page

func (a pageIndex) Len() int      { return len(a) }
func (a pageIndex) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a pageIndex) Less(i, j int) bool {
	return strings.ToLower(a[i].Title) < strings.ToLower(a[j].Title)
}
