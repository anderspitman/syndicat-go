package syndicat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-ap/activitypub"
	"github.com/go-ap/jsonld"
	"github.com/gorilla/feeds"
	"github.com/yuin/goldmark"
)

type WebFingerAccount struct {
	Subject string           `json:"subject"`
	Links   []*WebFingerLink `json:"links"`
}

type WebFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

type ActivityPubActor struct {
	Context []string `json:"@context"`
	Id      string   `json:"id"`
	Type    string   `json:"type"`
	Inbox   string   `json:"inbox"`
	Outbox  string   `json:"outbox"`
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
	var outboxItems activitypub.ItemCollection

	for _, item := range dirItems {

		entryId := item.Name()
		entryDir := filepath.Join(entriesDir, entryId)
		entryTextPath := filepath.Join(entryDir, "index.json")

		entryBytes, err := os.ReadFile(entryTextPath)
		if err != nil {
			return err
		}

		var entry *activitypub.Object
		err = json.Unmarshal(entryBytes, &entry)
		if err != nil {
			return err
		}

		var contentHtmlBuf bytes.Buffer
		if err := goldmark.Convert(entry.Content[0].Value, &contentHtmlBuf); err != nil {
			return err
		}

		entryRenderDir := filepath.Join(serveDir, entryId)
		entryHtmlPath := filepath.Join(entryRenderDir, "index.html")

		err = os.MkdirAll(entryRenderDir, 0755)
		if err != nil {
			return err
		}

		contentHtml := string(contentHtmlBuf.Bytes())

		tmplData := struct {
			Entry       *activitypub.Object
			ContentHtml string
		}{
			Entry:       entry,
			ContentHtml: contentHtml,
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
		// TODO: put in separate metadata file?
		//if entry.VanityPath != "" {
		//	fragment = fmt.Sprintf("#%s", entry.VanityPath)
		//}
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

		author := activitypub.IRI(rootUri)
		if activitypub.IsIRI(entry.AttributedTo) && entry.AttributedTo != activitypub.IRI("") {
			fmt.Println("at", entry.AttributedTo)
			author = entry.AttributedTo.(activitypub.IRI)
		}

		feedItem := &feeds.Item{
			Title: string(entry.Name[0].Value),
			Author: &feeds.Author{
				Name: string(author),
			},
			Id: entryUri,
			Link: &feeds.Link{
				Href: entryUri,
			},
			Content: contentHtml,
			Updated: entry.Updated,
		}

		feedItems = append(feedItems, feedItem)
		outboxItems = append(outboxItems, entry)
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

	// TODO: we might want to uncomment this to provide raw markdown in
	// the JSON feed
	//for _, item := range jsonFeed.Items {
	//	item.ContentText = item.ContentHTML
	//	item.ContentHTML = ""
	//}

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

	apOutboxUri := fmt.Sprintf("https://%s/outbox", rootUri)
	apOutbox := activitypub.OrderedCollectionNew(activitypub.IRI(apOutboxUri))
	apOutbox.OrderedItems = outboxItems

	outboxJson, err := jsonld.WithContext(
		jsonld.IRI(activitypub.ActivityBaseURI),
	).Marshal(apOutbox)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(serveDir, "outbox"), outboxJson, 0644)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(serveDir, "inbox"), []byte{}, 0644)
	if err != nil {
		return err
	}

	wf := &WebFingerAccount{
		Subject: fmt.Sprintf("me@%s", rootUri),
		Links: []*WebFingerLink{
			&WebFingerLink{
				Rel:  "self",
				Type: "application/activity+json",
				Href: fmt.Sprintf("https://%s/ap.json", rootUri),
			},
		},
	}

	wfJsonBytes, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}

	wfPath := filepath.Join(serveDir, ".well-known", "webfinger")

	err = os.MkdirAll(filepath.Dir(wfPath), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(wfPath, wfJsonBytes, 0644)
	if err != nil {
		return err
	}

	apId := activitypub.IRI(fmt.Sprintf("https://%s/ap.json", rootUri))
	apActor := &activitypub.Actor{
		ID:     apId,
		URL:    activitypub.IRI(fmt.Sprintf("https://%s", rootUri)),
		Type:   "Person",
		Inbox:  activitypub.IRI(fmt.Sprintf("https://%s/inbox", rootUri)),
		Outbox: activitypub.IRI(fmt.Sprintf("https://%s/outbox", rootUri)),
		PreferredUsername: activitypub.NaturalLanguageValues{
			activitypub.LangRefValue{
				Value: []byte("me"),
			},
		},
		Name: activitypub.NaturalLanguageValues{
			activitypub.LangRefValue{
				Value: []byte("me"),
			},
		},
	}

	// See here: https://github.com/go-ap/activitypub/issues/11
	apProfileBytes, err := jsonld.WithContext(
		jsonld.IRI(activitypub.ActivityBaseURI),
	).Marshal(apActor)
	if err != nil {
		return err
	}

	apProfilePath := filepath.Join(serveDir, "ap.json")

	err = os.WriteFile(apProfilePath, apProfileBytes, 0644)
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
