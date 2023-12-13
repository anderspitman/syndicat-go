package syndicat

import (
	"bytes"
	"crypto/rsa"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/anderspitman/treemess-go"
	"github.com/cbroglie/mustache"
	"github.com/gemdrive/gemdrive-go"
	"github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/jsonld"
	"github.com/lastlogin-io/obligator"
	"github.com/yuin/goldmark"
)

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
	sourceDir := fsDir
	serveDir := fsDir
	//sourceDir := filepath.Join(fsDir, "source")
	//serveDir := filepath.Join(fsDir, "serve")

	gdConfig := &gemdrive.Config{
		Dirs: []string{fsDir},
		DomainMap: map[string]string{
			rootUri: fmt.Sprintf("/%s", rootUri),
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

	privPemPath := filepath.Join(sourceDir, rootUri, "private_key.pem")
	_, err = os.Stat(privPemPath)
	if err != nil {
		privKey, err := MakeRSAKey()
		if err != nil {
			log.Fatal(err)
		}

		err = SaveRSAKey(privPemPath, privKey)
		if err != nil {
			log.Fatal(err)
		}

	}

	privKey, err := LoadRSAKey(privPemPath)
	if err != nil {
		log.Fatal(err)
	}

	pubKeyId := fmt.Sprintf("https://%s/ap.jsonld#main-key", rootUri)

	apClient := client.New()
	apClient.SignFn(func(r *http.Request) error {
		err := sign(privKey, pubKeyId, r)
		if err != nil {
			return err
		}

		//fmt.Println(r.Host, r.URL.Path)
		//printJson(r.Header)
		return nil
	})

	httpClient := &http.Client{}

	partialProvider := &PartialProvider{}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		//printJson(r.URL)
		//printJson(r.Header)
		//body, err := io.ReadAll(r.Body)
		//if err != nil {
		//	w.WriteHeader(500)
		//	io.WriteString(w, err.Error())
		//	return
		//}
		//fmt.Println(string(body))

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

	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		uri := r.Form.Get("uri")

		req, err := http.NewRequest("GET", uri, nil)
		if err != nil {
			log.Fatal(err)
		}

		parsedUrl, err := url.Parse(uri)
		check(err)

		dateHeader := time.Now().UTC().Format(http.TimeFormat)

		printJson(req.Header)
		req.Header.Set("Accept", "application/activity+json")
		req.Header.Set("Date", dateHeader)
		req.Header.Set("Host", parsedUrl.Host)
		printJson(req.Header)

		err = sign(privKey, pubKeyId, req)
		check(err)

		printJson(req.Header)

		resp, err := httpClient.Do(req)
		check(err)

		printJson(req)
		fmt.Println(resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		check(err)
		fmt.Println(string(body))

		http.Redirect(w, r, "/entry-editor/", http.StatusSeeOther)
	})

	http.HandleFunc("/get-object", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		uri := activitypub.IRI(r.Form.Get("uri"))

		obj, err := getObject(apClient, uri)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		printJson(obj)

		http.Redirect(w, r, "/entry-editor/", http.StatusSeeOther)
	})

	http.HandleFunc("/get-tree", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		uri := activitypub.IRI(r.Form.Get("uri"))

		err := getTree(apClient, uri, 0)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		http.Redirect(w, r, "/entry-editor/", http.StatusSeeOther)
	})

	http.HandleFunc("/inbox", func(w http.ResponseWriter, r *http.Request) {
		host := getHost(r)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		fmt.Println(string(body))
		var act *activitypub.Activity
		err = json.Unmarshal(body, &act)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		//var obj *activitypub.Object
		//err = json.NewDecoder(r.Body).Decode(&obj)
		//if err != nil {
		//	w.WriteHeader(500)
		//	io.WriteString(w, err.Error())
		//	return
		//}

		fmt.Println("/inbox")
		printJson(act)

		switch act.Type {
		case activitypub.FollowType:
			followersPath := filepath.Join(serveDir, host, "followers.jsonld")

			followersBytes, err := os.ReadFile(followersPath)
			if err != nil {
				w.WriteHeader(500)
				io.WriteString(w, err.Error())
				return
			}

			var followers *activitypub.OrderedCollection
			err = json.Unmarshal(followersBytes, &followers)
			if err != nil {
				w.WriteHeader(500)
				io.WriteString(w, err.Error())
				return
			}

			// TODO: using GetID() because it was panicking with a weird error when type asserting
			// act.Actor.(activitypub.IRI)
			newFollower := act.Actor.GetID()

			for _, f := range followers.OrderedItems {
				follower := f.(activitypub.IRI)
				if newFollower == follower {
					// already exists, noop
					return
				}
			}

			followers.OrderedItems = append(followers.OrderedItems, newFollower)
			followers.TotalItems = followers.TotalItems + 1

			followersBytes, err = jsonld.WithContext(
				jsonld.IRI(activitypub.ActivityBaseURI),
			).Marshal(followers)
			if err != nil {
				fmt.Println(err.Error())
				w.WriteHeader(500)
				io.WriteString(w, err.Error())
				return
			}

			err = os.WriteFile(followersPath, followersBytes, 0644)
			if err != nil {
				fmt.Println(err.Error())
				w.WriteHeader(500)
				io.WriteString(w, err.Error())
				return
			}

			accept := &activitypub.Accept{
				Type:   activitypub.AcceptType,
				Object: act,
			}

			err = sendActivity(httpClient, privKey, pubKeyId, accept, "https://mastodon.social/inbox")
			if err != nil {
				fmt.Println(err.Error())
				w.WriteHeader(500)
				io.WriteString(w, err.Error())
				return
			}
		}
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

		entryPath := filepath.Join(entryDir, "entry.jsonld")

		timestamp := time.Now()

		entryUri := fmt.Sprintf("https://%s/%d/", host, entryId)
		entryJsonUri := fmt.Sprintf("%sentry.jsonld", entryUri)

		var contentHtmlBuf bytes.Buffer
		if err := goldmark.Convert([]byte(entryText), &contentHtmlBuf); err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		htmlLink := activitypub.LinkNew("", activitypub.LinkType)
		htmlLink.Href = activitypub.IRI(entryUri)
		htmlLink.MediaType = "text/html"

		to := activitypub.ItemCollection{
			activitypub.IRI("https://www.w3.org/ns/activitystreams#Public"),
		}

		followersId := activitypub.IRI(fmt.Sprintf("https://%s/followers.jsonld", host))
		cc := activitypub.ItemCollection{
			activitypub.IRI(followersId),
		}

		feedItem := &activitypub.Object{
			Type: activitypub.NoteType,
			ID:   activitypub.IRI(entryJsonUri),
			Name: activitypub.NaturalLanguageValues{
				activitypub.LangRefValue{
					Value: []byte(titleText),
				},
			},
			AttributedTo: activitypub.IRI(host),
			Content: activitypub.NaturalLanguageValues{
				activitypub.LangRefValue{
					Value: contentHtmlBuf.Bytes(),
				},
			},
			Source: activitypub.Source{
				Content: activitypub.NaturalLanguageValues{
					activitypub.LangRefValue{
						Value: []byte(entryText),
					},
				},
				MediaType: "text/markdown",
			},
			Published: timestamp,
			Updated:   timestamp,
			InReplyTo: activitypub.IRI(parentUri),
			URL:       htmlLink,
			To:        to,
			CC:        cc,
		}

		jsonEntry, err := jsonld.WithContext(
			jsonld.IRI(activitypub.ActivityBaseURI),
		).Marshal(feedItem)
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

		activityPath := filepath.Join(entryDir, "activity.jsonld")
		activityId := activitypub.IRI(fmt.Sprintf("%s%s", entryUri, "activity.jsonld"))
		activity := activitypub.ActivityNew(activityId, activitypub.CreateType, feedItem)
		activity.Actor = activitypub.IRI(fmt.Sprintf("https://%s/ap.jsonld", rootUri))
		activity.To = to
		activity.CC = cc
		activity.Published = feedItem.Published

		activityJsonBytes, err := jsonld.WithContext(
			jsonld.IRI(activitypub.ActivityBaseURI),
		).Marshal(activity)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		err = os.WriteFile(activityPath, activityJsonBytes, 0644)
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

		err = sendActivity(httpClient, privKey, pubKeyId, activity, "https://mastodon.social/inbox")
		if err != nil {
			fmt.Println(err.Error())
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
	//fmt.Println(string(d))
	err := quick.Highlight(os.Stdout, string(d)+"\n", "json", "terminal256", "monokai")
	if err != nil {
		fmt.Println(err.Error())
	}
}

func printJsonLd(data interface{}) {
	d, _ := jsonld.Marshal(data)
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

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func sendActivity(httpClient *http.Client, privKey *rsa.PrivateKey, pubKeyId string, activity *activitypub.Activity, uri string) error {

	fmt.Println("sendActivity")
	printJson(activity)

	activityJsonBytes, err := jsonld.WithContext(
		jsonld.IRI(activitypub.ActivityBaseURI),
	).Marshal(activity)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", uri, bytes.NewReader(activityJsonBytes))
	if err != nil {
		return err
	}

	parsedUrl, err := url.Parse(uri)
	if err != nil {
		return err
	}

	dateHeader := time.Now().UTC().Format(http.TimeFormat)

	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("Date", dateHeader)
	req.Header.Set("Host", parsedUrl.Host)

	err = sign(privKey, pubKeyId, req)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	fmt.Println(resp.StatusCode)

	return nil
}
