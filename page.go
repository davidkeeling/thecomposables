package thecomposables

import (
	"encoding/json"
	"html/template"
	"sort"
	"strings"

	"golang.org/x/net/context"

	"time"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

type page struct {
	Key          *datastore.Key `datastore:"-"`
	ID           string         `datastore:"-"`
	Title        string
	Body         []byte        `datastore:",noindex"`
	Markup       template.HTML `datastore:"-"`
	Versions     []version
	Categories   string
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

func pagesFromMemcache(c context.Context) (pageIndex, error) {
	val, err := memcache.Get(c, "pages")
	if err == memcache.ErrCacheMiss {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var pages pageIndex
	err = json.Unmarshal(val.Value, &pages)
	if err != nil {
		return nil, err
	}

	return pages, nil
}

func savePagesInMemcache(c context.Context, pages pageIndex) error {
	pageJSON, err := json.Marshal(pages)
	if err != nil {
		return err
	}

	return memcache.Set(c, &memcache.Item{
		Key:   "pages",
		Value: pageJSON,
	})
}

func pagesFromDatastore(c context.Context) (pageIndex, error) {
	var pages pageIndex
	t := datastore.NewQuery("Page").Ancestor(pageParentKey(c)).Run(c)
	for {
		var p page
		_, err := t.Next(&p)
		if err == datastore.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		p.ID = titleToID(p.Title)
		pages = append(pages, &p)
	}
	sort.Sort(pages)
	return pages, nil
}

func getPages(c context.Context) (pageIndex, error) {
	pages, err := pagesFromMemcache(c)
	if pages != nil {
		return pages, nil
	}
	if err != nil {
		log.Errorf(c, "Fetching pages from memcache: %s")
	}

	return pagesFromDatastore(c)
}

func clearPageCache(c context.Context) {
	log.Infof(c, "Clearing page cache")
	err := memcache.Delete(c, "pages")
	if err != nil {
		log.Errorf(c, "Resetting pages memcache: %s", err)
	}
}

func getCategories(pages pageIndex) map[string]pageIndex {
	categories := make(map[string]pageIndex)
	for _, page := range pages {
		if page.Categories == "" {
			categories["uncategorized"] = append(categories["uncategorized"], page)
			continue
		}
		cats := strings.Split(page.Categories, " ")
		for _, cat := range cats {
			categories[cat] = append(categories[cat], page)
		}
	}
	return categories
}
