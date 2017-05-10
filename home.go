package thecomposables

import (
	"html/template"
	"net/http"

	"google.golang.org/appengine/user"

	"google.golang.org/appengine"
)

type homepageData struct {
	Pages        pageIndex
	Introduction template.HTML
	Categories   map[string]pageIndex
	User         *user.User
	IsAdmin      bool
	Redirect     string
}

func getIntroduction(pages pageIndex) template.HTML {
	for _, p := range pages {
		if p.Title == "Introduction" {
			return getMarkup(p.Body)
		}
	}
	return ""
}

func home(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	pages, err := getPages(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	u := user.Current(c)
	err = templates.ExecuteTemplate(w, "index.html", homepageData{
		Pages:        pages,
		Categories:   getCategories(pages),
		Introduction: getIntroduction(pages),
		User:         u,
		IsAdmin:      u != nil && u.Admin,
		Redirect:     "/",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
