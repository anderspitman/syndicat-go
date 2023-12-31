package syndicat

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/jsonld"
)

type ActivityPubObject struct {
	Id        string               `json:"id"`
	Name      string               `json:"name"`
	Content   string               `json:"content"`
	InReplyTo string               `json:"in_reply_to"`
	Replies   []*ActivityPubObject `json:"replies"`
	HtmlUri   string               `json:"html_uri"`
	UriName   string               `json:"uri_name"`
}

func convertApObjects(from []*activitypub.Object) ([]*ActivityPubObject, error) {
	to := []*ActivityPubObject{}
	for _, fromObj := range from {
		toObj, err := convertApObject(fromObj)
		if err != nil {
			return nil, err
		}

		to = append(to, toObj)
	}

	return to, nil
}

func convertApObject(from *activitypub.Object) (*ActivityPubObject, error) {

	inReplyTo := ""

	if from.InReplyTo != nil {
		inReplyTo = string(from.InReplyTo.(activitypub.IRI))
	}

	htmlUri := ""

	url, err := activitypub.ToLink(from.URL)
	if err != nil {
		return nil, err
	}

	if url.MediaType == "text/html" {
		htmlUri = string(url.Href)
	}

	to := &ActivityPubObject{
		Id:        string(from.ID),
		HtmlUri:   htmlUri,
		Name:      string(from.Name.First().Value),
		Content:   string(from.Content.First().Value),
		InReplyTo: inReplyTo,
		Replies:   []*ActivityPubObject{},
	}

	if from.Replies != nil {
		replies, err := activitypub.ToOrderedCollection(from.Replies)
		if err != nil {
			return nil, err
		}

		//nestedReplyItems := activitypub.ItemCollection{}

		for _, reply := range replies.OrderedItems {

			child, err := activitypub.ToObject(reply)
			if err != nil {
				return nil, err
			}

			childTo, err := convertApObject(child)
			if err != nil {
				return nil, err
			}

			to.Replies = append(to.Replies, childTo)
		}
	}

	return to, nil
}

func getTree(apClient *client.C, uri activitypub.IRI, depth int) (*activitypub.Object, error) {

	//for i := 0; i < depth; i++ {
	//	fmt.Print("    ")
	//}

	//fmt.Println(uri, depth)

	obj, err := getObject(apClient, uri)
	if err != nil {
		return nil, err
	}

	if obj.Replies != nil {

		replies, err := activitypub.ToOrderedCollection(obj.Replies)
		if err != nil {
			return nil, err
		}

		nestedReplyItems := activitypub.ItemCollection{}

		for _, reply := range replies.OrderedItems {
			iri, err := getIri(apClient, reply)
			if err != nil {
				return nil, err
			}
			child, err := getTree(apClient, iri, depth+1)
			if err != nil {
				return nil, err
			}

			nestedReplyItems = append(nestedReplyItems, child)
		}

		obj.Replies = &activitypub.OrderedCollection{
			TotalItems:   uint(len(nestedReplyItems)),
			OrderedItems: nestedReplyItems,
		}
	}

	return obj, nil
}

func getObject(apClient *client.C, uri activitypub.IRI) (*activitypub.Object, error) {

	parsedUri, err := url.Parse(string(uri))
	if err != nil {
		return nil, err
	}

	cacheDir := "ap_cache"
	objCachePath := filepath.Join(cacheDir, parsedUri.Host, parsedUri.Path)
	objCacheDir := filepath.Dir(objCachePath)

	err = os.MkdirAll(objCacheDir, 0755)
	if err != nil {
		return nil, err
	}

	objCacheBytes, err := os.ReadFile(objCachePath)
	if err != nil {
		// noop
	} else {
		var cachedObj *activitypub.Object
		err = jsonld.Unmarshal(objCacheBytes, &cachedObj)
		if err != nil {
			return nil, err
		}

		return cachedObj, nil
	}

	//fmt.Println("not cached", uri)

	ctx := context.Background()

	item, err := apClient.CtxLoadIRI(ctx, uri)
	if err != nil {
		placeholderObj := &activitypub.Object{
			Type: activitypub.NoteType,
			Content: activitypub.NaturalLanguageValues{
				activitypub.LangRefValue{
					Value: []byte("Failed to load entry"),
				},
			},
		}

		objWriteBytes, err := jsonld.Marshal(placeholderObj)
		if err != nil {
			return nil, err
		}

		err = os.WriteFile(objCachePath, []byte(objWriteBytes), 0644)
		if err != nil {
			return nil, err
		}

		return placeholderObj, err
	}

	obj, err := activitypub.ToObject(item)
	if err != nil {
		return nil, err
	}

	if obj.Replies != nil {

		allReplies := activitypub.OrderedCollectionNew("fakeid")

		replies, err := activitypub.ToCollection(obj.Replies)
		if err != nil {
			return nil, err
		}

		repliesPage, err := activitypub.ToCollectionPage(replies.First)
		if err != nil {
			return nil, err
		}

		for _, reply := range repliesPage.Items {
			iri, err := getIri(apClient, reply)
			if err != nil {
				return nil, err
			}
			allReplies.OrderedItems = append(allReplies.OrderedItems, iri)
		}

		next := repliesPage.Next.(activitypub.IRI)

		for {
			nextPageItem, err := apClient.CtxLoadIRI(ctx, next)
			if err != nil {
				fmt.Println(err.Error())
				break
				//return nil, err
			}

			nextPage, err := activitypub.ToCollectionPage(nextPageItem)
			if err != nil {
				return nil, err
			}

			for _, reply := range nextPage.Items {
				iri, err := getIri(apClient, reply)
				if err != nil {
					return nil, err
				}
				allReplies.OrderedItems = append(allReplies.OrderedItems, iri)
			}

			if nextPage.Next != nil {
				next = nextPage.Next.(activitypub.IRI)
			} else {
				break
			}
		}

		allReplies.TotalItems = uint(len(allReplies.OrderedItems))
		obj.Replies = allReplies

	}

	objWriteBytes, err := jsonld.Marshal(obj)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(objCachePath, []byte(objWriteBytes), 0644)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func getIri(apClient *client.C, item activitypub.Item) (activitypub.IRI, error) {
	var iri activitypub.IRI
	if item.IsLink() {
		iri = item.(activitypub.IRI)
	} else {
		obj, err := activitypub.ToObject(item)
		if err != nil {
			return "", err
		}

		iri = obj.ID
	}

	return iri, nil
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

func filterRootEntries(entries []*activitypub.Object) []*activitypub.Object {

	rootEntries := []*activitypub.Object{}

	for _, entry := range entries {
		if entry.InReplyTo == nil {
			rootEntries = append(rootEntries, entry)
		}
	}

	return rootEntries
}

func getAllEntries(entriesDir string) ([]*activitypub.Object, error) {

	dirItems, err := os.ReadDir(entriesDir)
	if err != nil {
		return nil, err
	}

	entries := []*activitypub.Object{}

	for _, item := range dirItems {

		entryIdStr := item.Name()

		_, err := strconv.Atoi(entryIdStr)
		if err != nil {
			continue
		}

		entryDir := filepath.Join(entriesDir, entryIdStr)
		entryTextPath := filepath.Join(entryDir, "activity.jsonld")

		activityBytes, err := os.ReadFile(entryTextPath)
		if err != nil {
			return nil, err
		}

		var activityItem *activitypub.Activity
		err = json.Unmarshal(activityBytes, &activityItem)
		if err != nil {
			return nil, err
		}

		activity, err := activitypub.ToActivity(activityItem)
		if err != nil {
			return nil, err
		}

		entry, err := activitypub.ToObject(activity.Object)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
