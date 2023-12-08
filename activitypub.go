package syndicat

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/go-ap/activitypub"
	"github.com/go-ap/client"
	"github.com/go-ap/jsonld"
)

func getTree(apClient *client.C, uri activitypub.IRI, depth int) error {

	for i := 0; i < depth; i++ {
		fmt.Print("    ")
	}

	fmt.Println(uri, depth)

	obj, err := getObject(apClient, uri)
	if err != nil {
		return err
	}

	replies, err := activitypub.ToOrderedCollection(obj.Replies)
	if err != nil {
		return err
	}

	for _, reply := range replies.OrderedItems {
		iri, err := getIri(apClient, reply)
		if err != nil {
			return err
		}
		err = getTree(apClient, iri, depth+1)
	}

	return nil
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
		// Noop
	} else {
		var cachedObj *activitypub.Object
		err = jsonld.Unmarshal(objCacheBytes, &cachedObj)
		if err != nil {
			return nil, err
		}

		return cachedObj, nil
	}

	fmt.Println("not cached", uri)

	ctx := context.Background()

	item, err := apClient.CtxLoadIRI(ctx, uri)
	if err != nil {
		return nil, err
	}
	fmt.Println("here1")

	obj, err := activitypub.ToObject(item)
	if err != nil {
		return nil, err
	}

	fmt.Println("here1.1")
	allReplies := activitypub.OrderedCollectionNew("fakeid")

	fmt.Println("here1.2")
	replies, err := activitypub.ToCollection(obj.Replies)
	if err != nil {
		return nil, err
	}

	fmt.Println("here1.3")
	repliesPage, err := activitypub.ToCollectionPage(replies.First)
	if err != nil {
		return nil, err
	}

	fmt.Println("here1.4")
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
			return nil, err
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

	objWriteBytes, err := jsonld.Marshal(obj)
	if err != nil {
		return nil, err
	}

	fmt.Println("here2")
	err = os.WriteFile(objCachePath, []byte(objWriteBytes), 0644)
	if err != nil {
		return nil, err
	}

	fmt.Println("here3")
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
