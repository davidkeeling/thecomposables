package thecomposables

import (
	"html/template"
	"strings"

	"golang.org/x/net/context"

	"time"

	"google.golang.org/appengine/datastore"
)

type page struct {
	Key          *datastore.Key `datastore:"-"`
	ID           string         `datastore:"-"`
	Title        string
	Body         []byte        `datastore:",noindex"`
	Markup       template.HTML `datastore:"-"`
	Versions     []version
	DoesNotExist bool `datastore:"-"`
}

type version struct {
	Body   []byte        `datastore:",noindex"`
	Markup template.HTML `datastore:"-"`
	Date   time.Time
}

func loadPage(c context.Context, title string) (*page, error) {
	var p page
	k := titleToKey(c, title)
	err := datastore.Get(c, k, &p)
	if err == datastore.ErrNoSuchEntity {
		p = page{
			Title:        title,
			DoesNotExist: true,
		}
	} else if err != nil {
		return nil, err
	}
	p.ID = titleToID(p.Title)
	p.Key = k

	return &p, nil
}

func (p *page) save(c context.Context) error {
	_, err := datastore.Put(c, p.Key, p)
	return err
}

func pageParentKey(c context.Context) *datastore.Key {
	return datastore.NewKey(c, "Topics", "default", 0, nil)
}

func titleToKey(c context.Context, title string) *datastore.Key {
	return datastore.NewKey(c, "Page", title, 0, pageParentKey(c))
}

// pageIndex implements alphabetical sort by Title for []*page
type pageIndex []*page

func (a pageIndex) Len() int      { return len(a) }
func (a pageIndex) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a pageIndex) Less(i, j int) bool {
	return strings.ToLower(a[i].Title) < strings.ToLower(a[j].Title)
}
