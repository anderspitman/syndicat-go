package syndicat

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	//"strings"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anderspitman/treemess-go"
	"github.com/cbroglie/mustache"
	"github.com/gemdrive/gemdrive-go"
	"github.com/gorilla/feeds"
	"github.com/lastlogin-io/obligator"
	"github.com/yuin/goldmark"
	//"willnorris.com/go/webmention"
)

type Entry struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	ContentType   string   `json:"content_type"`
	Content       string   `json:"content"`
	PublishedTime string   `json:"published_time"`
	ModifiedTime  string   `json:"modified_time"`
	Tags          []string `json:"tags"`
	VanityPath    string   `json:"vanity_path"`
	Parent        string   `json:"parent"`
	Children      []string `json:"children"`
}

type ServerConfig struct {
	RootUri string
}

type Server struct{}

//go:embed templates
var fs embed.FS

type PartialProvider struct {
}

func (p *PartialProvider) Get(tmplPath string) (string, error) {

	tmplBytes, err := fs.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}

	return string(tmplBytes), nil
}

func NewServer(conf ServerConfig) *Server {

	rootUri := conf.RootUri
	authUri := "auth." + rootUri

	authConfig := obligator.ServerConfig{
		RootUri: "https://" + authUri,
	}

	authServer := obligator.NewServer(authConfig)

	fsDir := "files"
	sourceDir := filepath.Join(fsDir, "source")
	serveDir := filepath.Join(fsDir, "serve")

	gdConfig := &gemdrive.Config{
		Dirs: []string{fsDir},
		DomainMap: map[string]string{
			rootUri: fmt.Sprintf("/serve/%s", rootUri),
		},
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

	partialProvider := &PartialProvider{}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		host := getHost(r)

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

	http.HandleFunc("/entry-submit", func(w http.ResponseWriter, r *http.Request) {

		r.ParseForm()

		titleText := r.Form.Get("title")
		entryText := r.Form.Get("entry")

		host := getHost(r)

		userDir := filepath.Join(sourceDir, host)

		dirItems, err := os.ReadDir(userDir)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		lastId := 0

		for _, item := range dirItems {
			entryIdStr := item.Name()

			entryId, err := strconv.Atoi(entryIdStr)
			if err != nil {
				continue
			}

			if !item.IsDir() {
				continue
			}

			if entryId > lastId {
				lastId = entryId
			}
		}

		entryId := lastId + 1

		entryDir := fmt.Sprintf("%s/%d", userDir, entryId)
		err = os.MkdirAll(entryDir, 0755)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		entryPath := filepath.Join(entryDir, "index.json")

		timestamp := time.Now().Format(time.RFC3339)

		//feedItem := &feeds.JSONItem{
		//	Title: titleText,
		//	Author: &feeds.JSONAuthor{
		//		Name: "Me",
		//	},
		//	ContentText:   entryText,
		//	PublishedDate: &timestamp,
		//	ModifiedDate:  &timestamp,
		//}

		feedItem := &Entry{
			Title:         titleText,
			Author:        "Me",
			Content:       entryText,
			PublishedTime: timestamp,
			ModifiedTime:  timestamp,
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

		err = render(rootUri, sourceDir, serveDir, partialProvider)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		entryUriPath := fmt.Sprintf("/%d/", entryId)
		http.Redirect(w, r, entryUriPath, http.StatusSeeOther)
	})

	err = render(rootUri, sourceDir, serveDir, partialProvider)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	http.ListenAndServe(":9005", nil)

	s := &Server{}
	return s
}

func render(rootUri, sourceDir, serveDir string, partialProvider *PartialProvider) error {

	err := os.MkdirAll(sourceDir, 0755)
	if err != nil {
		return err
	}

	err = os.MkdirAll(serveDir, 0755)
	if err != nil {
		return err
	}

	dirItems, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	for _, userDirEntry := range dirItems {
		domainName := userDirEntry.Name()
		userRootUri := domainName
		userSourceDir := filepath.Join(sourceDir, domainName)
		userServeDir := filepath.Join(serveDir, domainName)
		err = renderUser(userRootUri, userSourceDir, userServeDir, partialProvider)
		if err != nil {
			return err
		}
	}

	return nil
}

func renderUser(rootUri, sourceDir, serveDir string, partialProvider *PartialProvider) error {

	err := os.MkdirAll(sourceDir, 0755)
	if err != nil {
		return err
	}

	err = os.MkdirAll(serveDir, 0755)
	if err != nil {
		return err
	}

	entriesDir := sourceDir

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

		var entry *Entry
		err = json.Unmarshal(entryBytes, &entry)
		if err != nil {
			return err
		}

		var contentText bytes.Buffer
		if err := goldmark.Convert([]byte(entry.Content), &contentText); err != nil {
			return err
		}

		entryRenderDir := filepath.Join(serveDir, entryId)
		entryHtmlPath := filepath.Join(entryRenderDir, "index.html")

		err = os.MkdirAll(entryRenderDir, 0755)
		if err != nil {
			return err
		}

		content := string(contentText.Bytes())

		tmplData := struct {
			Title   string
			Content string
		}{
			Title:   entry.Title,
			Content: content,
		}

		entryHtml, err := renderTemplate("templates/entry.html", tmplData, partialProvider)
		if err != nil {
			return err
		}

		//reader := strings.NewReader(entryHtml)

		//links, err := webmention.DiscoverLinksFromReader(reader, rootUri, ".content")
		//if err != nil {
		//	return err
		//}

		fragment := ""
		if entry.VanityPath != "" {
			fragment = fmt.Sprintf("#%s", entry.VanityPath)
		}
		entryUri := fmt.Sprintf("https://%s/%s/%s", rootUri, entryId, fragment)

		//wmClient := webmention.New(nil)

		//for _, link := range links {
		//	endpoint, err := wmClient.DiscoverEndpoint(link)
		//	if err != nil {
		//		return err
		//	}

		//	fmt.Println(endpoint, entryUri, link)
		//	wmClient.SendWebmention(endpoint, entryUri, link)
		//}

		err = os.WriteFile(entryHtmlPath, []byte(entryHtml), 0644)
		if err != nil {
			return err
		}

		modTime, err := time.Parse("2006-01-02T15:04:05-07:00", entry.ModifiedTime)
		if err != nil {
			return err
		}

		feedItem := &feeds.Item{
			Title: entry.Title,
			Author: &feeds.Author{
				Name: entry.Author,
			},
			Id: entryUri,
			Link: &feeds.Link{
				Href: entryUri,
			},
			Content: entry.Content,
			Updated: modTime,
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

	err = os.WriteFile(filepath.Join(serveDir, "feed.xml"), []byte(atom), 0644)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(serveDir, "feed.json"), []byte(feedJson), 0644)
	if err != nil {
		return err
	}

	templateData := struct {
		Title    string
		LoggedIn bool
	}{
		Title:    rootUri,
		LoggedIn: true,
	}

	indexHtml, err := renderTemplate("templates/index.html", templateData, partialProvider)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(serveDir, "index.html"), []byte(indexHtml), 0644)
	if err != nil {
		return err
	}

	blogTmplData := struct {
		Entries  []*feeds.Item
		LoggedIn bool
	}{
		Entries:  feedItems,
		LoggedIn: true,
	}

	blogHtml, err := renderTemplate("templates/blog.html", blogTmplData, partialProvider)
	if err != nil {
		return err
	}

	blogDir := filepath.Join(serveDir, "blog")

	err = os.MkdirAll(blogDir, 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(blogDir, "index.html"), []byte(blogHtml), 0644)
	if err != nil {
		return err
	}

	editorTmplData := struct {
		Title    string
		LoggedIn bool
	}{
		Title:    "Entree Entry",
		LoggedIn: true,
	}

	editorTmplHtml, err := renderTemplate("templates/entry-editor.html", editorTmplData, partialProvider)
	if err != nil {
		return err
	}

	editorDir := filepath.Join(serveDir, "entry-editor")

	err = os.MkdirAll(editorDir, 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(editorDir, "index.html"), []byte(editorTmplHtml), 0644)
	if err != nil {
		return err
	}

	return nil
}

func renderTemplate(tmplPath string, templateData interface{}, partialProvider *PartialProvider) (string, error) {

	tmplBytes, err := fs.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}

	tmplText, err := mustache.RenderPartials(string(tmplBytes), partialProvider, templateData)
	if err != nil {
		return "", err
	}

	return tmplText, nil
}

func printJson(data interface{}) {
	d, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(d))
}

func getHost(r *http.Request) string {
	// TODO: check to make sure we're behind a proxy before
	// trusting XFH header
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	return host
}
