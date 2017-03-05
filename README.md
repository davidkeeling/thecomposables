
This is a simple wiki based on [https://golang.org/doc/articles/wiki/](https://golang.org/doc/articles/wiki/), with a few changes: 

- Page content is parsed as Markdown
- Uses Google App Engine's datastore instead of saving to the file system
- 10 previous versions of each page are stored
- Implemented page deletion
- Locked down editing to admin users

Created for my scifi worldbuilding project [The Composables](http://thecomposables.com/), but it's general enough to quickly adapt for a wiki on any topic.
