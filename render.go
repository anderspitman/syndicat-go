package syndicat

import (
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"github.com/go-ap/activitypub"
	"github.com/go-ap/jsonld"
	"github.com/gorilla/feeds"
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

type PartialProvider struct {
	fs iofs.ReadFileFS
}

func NewPartialProvider(fs iofs.ReadFileFS) *PartialProvider {
	return &PartialProvider{
		fs: fs,
	}
}

func (p *PartialProvider) Get(tmplPath string) (string, error) {

	tmplBytes, err := p.fs.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}

	return string(tmplBytes), nil
}

func render(rootUri, sourceDir, serveDir string, partialProvider *PartialProvider) error {

	err := ensureDir(sourceDir)
	if err != nil {
		return err
	}

	err = ensureDir(serveDir)
	if err != nil {
		return err
	}

	dirItems, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	for _, userDirEntry := range dirItems {
		if !userDirEntry.IsDir() {
			continue
		}
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

	privKeyPath := filepath.Join(sourceDir, "private_key.pem")
	privKey, err := LoadRSAKey(privKeyPath)
	if err != nil {
		return err
	}

	publicKeyPem, err := GetPublicKeyPem(privKey)
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

		entryIdStr := item.Name()

		entryId, err := strconv.Atoi(entryIdStr)
		if err != nil {
			continue
		}

		entryDir := fmt.Sprintf("%s/%d", entriesDir, entryId)
		entryTextPath := filepath.Join(entryDir, "activity.jsonld")

		activityBytes, err := os.ReadFile(entryTextPath)
		if err != nil {
			return err
		}

		var activityItem *activitypub.Activity
		err = json.Unmarshal(activityBytes, &activityItem)
		if err != nil {
			return err
		}

		activity, err := activitypub.ToActivity(activityItem)
		if err != nil {
			return err
		}

		entry, err := activitypub.ToObject(activity.Object)
		if err != nil {
			return err
		}

		entryRenderDir := fmt.Sprintf("%s/%d", serveDir, entryId)
		entryHtmlPath := filepath.Join(entryRenderDir, "index.html")

		err = os.MkdirAll(entryRenderDir, 0755)
		if err != nil {
			return err
		}

		contentHtml := string(entry.Content.First().Value)

		renderEntry, err := convertApObject(entry)
		if err != nil {
			return err
		}

		tmplData := struct {
			Entry       *ActivityPubObject
			ContentHtml string
			LoggedIn    bool
		}{
			Entry:       renderEntry,
			ContentHtml: contentHtml,
			LoggedIn:    true,
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
		entryUri := fmt.Sprintf("https://%s/%d/%s", rootUri, entryId, fragment)

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
		outboxItems = append(outboxItems, activity)
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

	apOutboxUri := fmt.Sprintf("https://%s/outbox.jsonld", rootUri)
	apOutbox := activitypub.OrderedCollectionNew(activitypub.IRI(apOutboxUri))
	apOutbox.OrderedItems = outboxItems
	apOutbox.TotalItems = uint(len(outboxItems))

	outboxJson, err := jsonld.WithContext(
		jsonld.IRI(activitypub.ActivityBaseURI),
	).Marshal(apOutbox)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(serveDir, "outbox.jsonld"), outboxJson, 0644)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(serveDir, "inbox"), []byte{}, 0644)
	if err != nil {
		return err
	}

	wf := &WebFingerAccount{
		Subject: fmt.Sprintf("me17@%s", rootUri),
		Links: []*WebFingerLink{
			&WebFingerLink{
				Rel:  "self",
				Type: "application/activity+json",
				Href: fmt.Sprintf("https://%s/ap.jsonld", rootUri),
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

	actorId := activitypub.IRI(fmt.Sprintf("https://%s/ap.jsonld", rootUri))
	pubKeyId := actorId + "#main-key"
	apActor := &activitypub.Actor{
		ID:  actorId,
		URL: activitypub.IRI(fmt.Sprintf("https://%s", rootUri)),
		Icon: activitypub.Image{
			Type:      activitypub.ImageType,
			MediaType: "image/jpeg",
			URL:       activitypub.IRI(fmt.Sprintf("https://%s/portrait.jpg", rootUri)),
		},
		Type:      "Person",
		Inbox:     activitypub.IRI(fmt.Sprintf("https://%s/inbox", rootUri)),
		Outbox:    activitypub.IRI(fmt.Sprintf("https://%s/outbox.jsonld", rootUri)),
		Followers: activitypub.IRI(fmt.Sprintf("https://%s/followers.jsonld", rootUri)),
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
		PublicKey: activitypub.PublicKey{
			ID:           pubKeyId,
			Owner:        actorId,
			PublicKeyPem: publicKeyPem,
		},
	}

	// See here: https://github.com/go-ap/activitypub/issues/11
	apProfileBytes, err := jsonld.WithContext(
		jsonld.IRI(activitypub.ActivityBaseURI),
	).Marshal(apActor)
	if err != nil {
		return err
	}

	apProfilePath := filepath.Join(serveDir, "ap.jsonld")

	err = os.WriteFile(apProfilePath, apProfileBytes, 0644)
	if err != nil {
		return err
	}

	followersPath := filepath.Join(serveDir, "followers.jsonld")

	followersId := activitypub.IRI(fmt.Sprintf("https://%s/followers.jsonld", rootUri))
	followersBytes, err := os.ReadFile(followersPath)
	if err != nil {
		followers := activitypub.OrderedCollectionNew(followersId)
		followersBytes, err = jsonld.WithContext(
			jsonld.IRI(activitypub.ActivityBaseURI),
		).Marshal(followers)
		if err != nil {
			return err
		}

		err = os.WriteFile(followersPath, followersBytes, 0644)
		if err != nil {
			return err
		}
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

	err = ensureDirWriteFile(filepath.Join(editorDir, "index.html"), []byte(editorTmplHtml))
	if err != nil {
		return err
	}

	allEntries, err := getAllEntries(entriesDir)
	if err != nil {
		return err
	}

	forumDir := filepath.Join(sourceDir, "forum")
	err = renderForum(forumDir, allEntries, partialProvider)
	if err != nil {
		return err
	}

	err = renderBlog(feedItems, serveDir, partialProvider)
	if err != nil {
		return err
	}

	return nil
}

func renderBlog(feedItems []*feeds.Item, serveDir string, partialProvider *PartialProvider) error {

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

	err = ensureDirWriteFile(filepath.Join(blogDir, "index.html"), []byte(blogHtml))
	if err != nil {
		return err
	}

	return nil
}

func renderForum(dstDir string, allEntries []*activitypub.Object, partialProvider *PartialProvider) error {

	ensureDir(dstDir)

	forumEntries := filterRootEntries(allEntries)

	renderEntries, err := convertApObjects(forumEntries)
	if err != nil {
		return err
	}

	for _, entry := range renderEntries {

		entry.UriName = strings.Replace(strings.ToLower(entry.Name), ".", "-", -1)

		replies := []*activitypub.Object{}

		// TODO: this can be more efficient by making a map of categories and looping through once
		for _, maybeReply := range allEntries {
			if maybeReply.InReplyTo != nil {
				inReplyToUri := string(maybeReply.InReplyTo.(activitypub.IRI))

				fmt.Println(inReplyToUri, entry.Id)
				if inReplyToUri == entry.Id {
					replies = append(replies, maybeReply)
				}
			}
		}

		renderReplies, err := convertApObjects(replies)
		if err != nil {
			return err
		}

		fmt.Println("replies")
		printJson(replies)
		fmt.Println("render replies")
		printJson(renderReplies)

		tmplData := struct {
			Entry   *ActivityPubObject
			Replies []*ActivityPubObject
		}{
			Entry:   entry,
			Replies: renderReplies,
		}

		dstPath := filepath.Join(dstDir, "c", entry.UriName, "index.html")
		err = renderTemplateToFile("templates/forum/category.html", dstPath, tmplData, partialProvider)
		if err != nil {
			return err
		}
	}

	tmplData := struct {
		Entries []*ActivityPubObject
	}{
		Entries: renderEntries,
	}

	dstPath := filepath.Join(dstDir, "index.html")
	err = renderTemplateToFile("templates/forum/index.html", dstPath, tmplData, partialProvider)
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

func renderTemplateToFile(tmplPath, dstPath string, templateData interface{}, partialProvider *PartialProvider) error {
	str, err := renderTemplate(tmplPath, templateData, partialProvider)
	if err != nil {
		return err
	}

	return ensureDirWriteFile(dstPath, []byte(str))
}
