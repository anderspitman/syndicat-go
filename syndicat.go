package syndicat

import (
	"embed"
	"encoding/json"
	"fmt"
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
	"github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/lastlogin-io/obligator"
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
	ParentUri     string   `json:"parent_uri"`
	ChildrenUris  []string `json:"children_uris"`
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

	apClient := client.New()

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

	http.HandleFunc("/get-post", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		uri := activitypub.IRI(r.Form.Get("uri"))

		//obj, err := getObject(apClient, uri)
		err := getTree(apClient, uri, 0)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		//fmt.Println("obj")
		//printJson(obj)

		http.Redirect(w, r, "/entry-editor/", http.StatusSeeOther)
	})

	http.HandleFunc("/entry-submit", func(w http.ResponseWriter, r *http.Request) {

		r.ParseForm()

		titleText := r.Form.Get("title")
		entryText := r.Form.Get("entry")
		parentUri := r.Form.Get("parent_uri")

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
			Author:        host,
			Content:       entryText,
			PublishedTime: timestamp,
			ModifiedTime:  timestamp,
			ParentUri:     parentUri,
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
