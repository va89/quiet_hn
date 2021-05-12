package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"quiet_hn/hn"
)

// for range time.Tick(40 * time.Second) {

// Stories strtuct
type Stories struct {
	sync.RWMutex
	Stories *[]item
}

// NewStories constructor
func NewStories() *Stories {
	return &Stories{
		RWMutex: sync.RWMutex{},
		Stories: &[]item{},
	}
}

// Length returns Stories len in a thread safe way
func (st *Stories) Length() int {
	st.Lock()
	defer st.Unlock()
	len := len(*st.Stories)
	return len
}

// Emptify removes items from Stories.stories in a thread safe way
func (st *Stories) Emptify() {
	var emptyStories []item
	st.Lock()
	defer st.Unlock()
	*st.Stories = emptyStories
}

// Reassign assign stories from another Stories in a thread safe way
func (st *Stories) Reassign(newSt *Stories) {
	st.Lock()
	defer st.Unlock()
	*st.Stories = *newSt.Stories
}

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	stories := NewStories()

	go updateCache(stories)
	// go storiesCacheInvalidator(stories)
	http.HandleFunc("/", handler(stories, numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func storiesCacheInvalidator(stories *Stories) {
	for {
		select {
		case <-time.After(time.Duration(40 * time.Second)):
			fmt.Println("Invalidate stories cache")
			stories.Emptify()
		}
	}
}

func handler(stories *Stories, numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		stories.Lock()
		defer stories.Unlock()
		renderTemplate(stories, start, tpl, w)
	})
}

// func updateItems(w http.ResponseWriter, stories *Stories, c1 chan int) {
// 	if stories.Length() >= 30 {
// 		c1 <- 1
// 		return
// 	}

// 	var client hn.Client
// 	ids, err := client.TopItems()
// 	if err != nil {
// 		http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
// 		return
// 	}

// 	for i, id := range ids {
// 		go collectStories(i, id, client, stories, c1)
// 	}

// 	go trackItemsCounter(stories, c1)
// }

func updateCache(stories *Stories) {
	for {
		storiesIsFullCh := make(chan int, 30)
		var client hn.Client
		newStories := NewStories()
		ids, err := client.TopItems()

		fmt.Println("Updating cache")
		if err != nil {
			fmt.Println("Failed to load top stories in cache")
			return
		}

		stopChan := make(chan int, 1)

		go trackItemsCounter(newStories, storiesIsFullCh)

		chunk := 30
		begin := 0
		next := chunk

		for newStories.Length() < 30 {
			var wg sync.WaitGroup
			fmt.Println(begin, next)

			if next >= len(ids) {
				break
			}

			for i, id := range ids[begin:next] {
				wg.Add(1)
				go collectStories(&wg, i+begin, id, client, newStories, storiesIsFullCh)
			}

			wg.Wait()
			fmt.Println("all executed")
			fmt.Println(len(*newStories.Stories))
			begin = next
			next += chunk

		}

		select {
		case <-storiesIsFullCh:
			newStories.Lock()
			stopChan <- 0
			sort.Slice(*newStories.Stories, func(i, j int) bool {
				return (*newStories.Stories)[i].Item.Order < (*newStories.Stories)[j].Item.Order
			})
			stories.Reassign(newStories)
			newStories.Unlock()
			time.Sleep(time.Duration(time.Second * 60))
		case <-time.After(time.Duration(5 * time.Second)):
			fmt.Println("Failed to update cache")
		}
	}
}

func renderTemplate(stories *Stories, start time.Time, tpl *template.Template, w http.ResponseWriter) {
	data := templateData{
		Stories: *stories.Stories,
		Time:    time.Now().Sub(start),
	}

	err := tpl.Execute(w, data)

	if err != nil {
		http.Error(w, "Failed to process the template", http.StatusInternalServerError)
		return
	}
}

func trackItemsCounter(stories *Stories, c1 chan int) {
	for stories.Length() < 30 {
		time.Sleep(time.Duration(time.Microsecond * 10))
	}
	c1 <- 1
}

func collectStories(wg *sync.WaitGroup, order int, id int, client hn.Client, stories *Stories, c1 chan int) {
	defer wg.Done()
	fmt.Println(order)
	// fmt.Println(order)
	if stories.Length() >= 30 {
		// c1 <- 1
		return
	}

	hnItem, err := client.GetItem(id)

	if err != nil {
		return
	}

	item := parseHNItem(hnItem)
	// fmt.Println(order)
	if isStoryLink(item) {
		if stories.Length() >= 30 {
			// c1 <- 1
			return
		}
		item.Order = order
		stories.Lock()
		// fmt.Println(len(*stories.Stories))
		*stories.Stories = append(*stories.Stories, item)
		stories.Unlock()
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
