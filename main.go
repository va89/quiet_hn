package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"quiet_hn/hn"
)

type count32 int32

func (c *count32) inc() int32 {
	return atomic.AddInt32((*int32)(c), 1)
}

func (c *count32) get() int32 {
	return atomic.LoadInt32((*int32)(c))
}

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var client hn.Client
		ids, err := client.TopItems()
		if err != nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}

		var stories []item
		c1 := make(chan int, 1)

		for _, id := range ids {
			go collectStories(id, client, &stories, c1)
		}

		go trackItemsCounter(&stories, c1)
		// sort.Slice(stories, func(i, j int) bool {
		// 	return stories[i].ID > stories[j].ID
		// })

		select {
		case <-c1:
			renderTemplate(stories, start, tpl, w)
		case <-time.After(time.Duration(5 * time.Second)):
			renderTemplate(stories, start, tpl, w)
		}
	})
}

func renderTemplate(stories []item, start time.Time, tpl *template.Template, w http.ResponseWriter) {
	data := templateData{
		Stories: stories,
		Time:    time.Now().Sub(start),
	}
	err := tpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to process the template", http.StatusInternalServerError)
		return
	}
}

func trackItemsCounter(stories *[]item, c1 chan int) {
	for len(*stories) < 30 {
		time.Sleep(time.Duration(time.Millisecond * 10))
	}
	c1 <- 1
}

func collectStories(id int, client hn.Client, stories *[]item, c1 chan int) {
	hnItem, err := client.GetItem(id)

	if err != nil {
		return
	}

	item := parseHNItem(hnItem)

	if isStoryLink(item) {
		if len(*stories) >= 30 {
			c1 <- 0
			return
		}
		*stories = append(*stories, item)
	}
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
