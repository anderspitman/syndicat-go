package syndicat

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anderspitman/treemess-go"
	"github.com/gemdrive/gemdrive-go"
	"github.com/gorilla/feeds"
	"github.com/lastlogin-io/obligator"
	"github.com/yuin/goldmark"
)

type ServerConfig struct {
	RootUri string
}

type Server struct{}

//go:embed templates
var fs embed.FS

func NewServer(conf ServerConfig) *Server {

	rootUri := conf.RootUri
	authUri := "auth." + rootUri

	authConfig := obligator.ServerConfig{
		RootUri: "https://" + authUri,
	}

	authServer := obligator.NewServer(authConfig)

	fsDir := "files"

	gdConfig := &gemdrive.Config{
		Dirs: []string{fsDir},
	}

	tmess := treemess.NewTreeMess()
	gdTmess := tmess.Branch()

	gdServer, err := gemdrive.NewServer(gdConfig, gdTmess)
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan treemess.Message)
	tmess.Listen(ch)

	//tmess.Send("start", nil)

	go func() {
		for msg := range ch {
			fmt.Println(msg)
		}
	}()

	tmpl, err := template.ParseFS(fs, "templates/*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	nextId := 0
	mut := &sync.Mutex{}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		// TODO: check to make sure we're behind a proxy before
		// trusting XFH header
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}

		switch host {
		case rootUri:

			//validation, err := authServer.Validate(r)
			//if err != nil {
			//	redirectUri := "https://" + rootUri
			//	url := fmt.Sprintf("https://%s/auth?client_id=%s&redirect_uri=%s&response_type=code&state=&scope=",
			//		authUri, redirectUri, redirectUri)
			//	http.Redirect(w, r, url, 307)
			//	return
			//}
			gdServer.ServeHTTP(w, r)
			return
		case authUri:
			authServer.ServeHTTP(w, r)
			return
		}
	})

	http.HandleFunc("/entry", func(w http.ResponseWriter, r *http.Request) {

		templateData := struct {
		}{}

		err = tmpl.ExecuteTemplate(w, "entry.html", templateData)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}
	})

	http.HandleFunc("/entry-submit", func(w http.ResponseWriter, r *http.Request) {

		r.ParseForm()

		titleText := r.Form.Get("title")
		entryText := r.Form.Get("entry")

		entryId := nextId
		mut.Lock()
		nextId++
		mut.Unlock()

		entryDir := fmt.Sprintf("%s/entries/%d/", fsDir, entryId)
		err = os.MkdirAll(entryDir, 0755)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		entryPath := filepath.Join(entryDir, "index.json")

		timestamp := time.Now()

		feedItem := &feeds.JSONItem{
			Title: titleText,
			Author: &feeds.JSONAuthor{
				Name: "Me",
			},
			ContentText:   entryText,
			PublishedDate: &timestamp,
			ModifiedDate:  &timestamp,
		}

		jsonEntry, err := json.MarshalIndent(feedItem, "", "  ")
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		err = os.WriteFile(entryPath, jsonEntry, 0644)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		err = render(rootUri, fsDir, fsDir)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		http.Redirect(w, r, "/entry", http.StatusSeeOther)
	})

	err = render(rootUri, fsDir, fsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	http.ListenAndServe(":9005", nil)

	s := &Server{}
	return s
}

func render(rootUri, fsDir, renderDir string) error {

	entriesDir := filepath.Join(fsDir, "entries")

	dirItems, err := os.ReadDir(entriesDir)
	if err != nil {
		return err
	}

	feedItems := []*feeds.Item{}

	for _, item := range dirItems {

		entryId := item.Name()
		entryDir := filepath.Join(entriesDir, entryId)
		entryTextPath := filepath.Join(entryDir, "index.json")

		entryBytes, err := os.ReadFile(entryTextPath)
		if err != nil {
			return err
		}

		var entry *feeds.JSONItem
		err = json.Unmarshal(entryBytes, &entry)
		if err != nil {
			return err
		}

		printJson(entry)

		var htmlText bytes.Buffer
		if err := goldmark.Convert([]byte(entry.ContentText), &htmlText); err != nil {
			return err
		}

		entryRenderDir := filepath.Join(renderDir, entryId)
		entryHtmlPath := filepath.Join(entryRenderDir, "index.html")

		err = os.MkdirAll(entryRenderDir, 0755)
		if err != nil {
			return err
		}

		err = os.WriteFile(entryHtmlPath, htmlText.Bytes(), 0644)
		if err != nil {
			return err
		}

		feedItem := &feeds.Item{
			Title: entry.Title,
			Author: &feeds.Author{
				Name: entry.Author.Name,
			},
			Id: fmt.Sprintf("https://%s/%s/", rootUri, entryId),
			Link: &feeds.Link{
				Href: fmt.Sprintf("https://%s/%s/", rootUri, entryId),
			},
			Content: entry.ContentText,
			Updated: *entry.ModifiedDate,
		}

		feedItems = append(feedItems, feedItem)
	}

	feed := &feeds.Feed{
		Title: "IndieHost.org feed",
		Link: &feeds.Link{
			Href: fmt.Sprintf("https://%s/feed.xml", rootUri),
			Rel:  "self",
		},
		Items:   feedItems,
		Updated: time.Now(),
	}

	atom, err := feed.ToAtom()
	if err != nil {
		return err
	}

	jsonFeed := (&feeds.JSON{Feed: feed}).JSONFeed()
	jsonFeed.HomePageUrl = fmt.Sprintf("https://%s/", rootUri)
	jsonFeed.FeedUrl = fmt.Sprintf("https://%s/feed.json", rootUri)
	for _, item := range jsonFeed.Items {
		item.ContentText = item.ContentHTML
		item.ContentHTML = ""
	}

	feedJson, err := jsonFeed.ToJSON()
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(fsDir, "feed.xml"), []byte(atom), 0644)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(fsDir, "feed.json"), []byte(feedJson), 0644)
	if err != nil {
		return err
	}

	return nil
}

func printJson(data interface{}) {
	d, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(d))
}
