package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-ap/activitypub"
	"github.com/go-ap/jsonld"
	"github.com/yuin/goldmark"
	//"github.com/anderspitman/syndicat-go"
)

type LegacyEntry struct {
	Title           string   `json:"title"`
	Format          string   `json:"format"`
	ContentFilename string   `json:"contentFilename"`
	Timestamp       string   `json:"timestamp"`
	Id              int      `json:"id"`
	Tags            []string `json:"tags"`
	Keywords        []string `json:"keywords"`
	UrlName         string   `json:"urlName"`
	Visibility      string   `json:"visibility"`
}

func main() {
	srcDirArg := flag.String("src-dir", "./src", "Source directory")
	dstDirArg := flag.String("dst-dir", "./dst", "Destination directory")
	//authorArg := flag.String("author", "", "Author")
	domain := flag.String("domain", "", "Domain")
	flag.Parse()

	host := *domain

	srcDir := *srcDirArg
	dstDir := *dstDirArg
	//author := *authorArg

	dirItems, err := os.ReadDir(srcDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	for _, item := range dirItems {
		entryIdStr := item.Name()

		entryId, err := strconv.Atoi(entryIdStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			continue
		}

		if !item.IsDir() {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		entryJsonPath := filepath.Join(srcDir, entryIdStr, "entry.json")
		fmt.Println(entryJsonPath)

		entryJsonBytes, err := os.ReadFile(entryJsonPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		var legacyEntry LegacyEntry

		err = json.Unmarshal(entryJsonBytes, &legacyEntry)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		contentFilename := "entry.md"
		if legacyEntry.ContentFilename != "" {
			contentFilename = legacyEntry.ContentFilename
		}

		contentPath := filepath.Join(srcDir, entryIdStr, contentFilename)
		contentBytes, err := os.ReadFile(contentPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		//timestamp := legacyEntry.Timestamp
		//if len(timestamp) == 20 {
		//	timestamp = timestamp[:19] + "+00:00"
		//} else if len(timestamp) == 10 {
		//	timestamp = timestamp + "T00:00:00+00:00"
		//}
		// TODO: convert actual timestamps

		timestamp := time.Now()

		fmt.Println(legacyEntry.Timestamp, timestamp)
		fmt.Println(len(contentBytes))

		var contentHtmlBuf bytes.Buffer
		err = goldmark.Convert(contentBytes, &contentHtmlBuf)
		exitOnErr(err)

		entryUri := fmt.Sprintf("https://%s/%d/", host, entryId)
		entryJsonUri := fmt.Sprintf("%sentry.jsonld", entryUri)

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

		entry := &activitypub.Object{
			Type: activitypub.ArticleType,
			ID:   activitypub.IRI(entryJsonUri),
			Name: activitypub.NaturalLanguageValues{
				activitypub.LangRefValue{
					Value: []byte(legacyEntry.Title),
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
						Value: contentBytes,
					},
				},
				MediaType: "text/markdown",
			},
			Published: timestamp,
			Updated:   timestamp,
			//InReplyTo: activitypub.IRI(parentUri),
			URL: htmlLink,
			To:  to,
			CC:  cc,
			Tag: activitypub.ItemCollection{},
		}

		//entry := &syndicat.Entry{
		//	Title:         legacyEntry.Title,
		//	Author:        author,
		//	PublishedTime: timestamp,
		//	ModifiedTime:  timestamp,
		//	Content:       string(contentBytes),
		//	VanityPath:    legacyEntry.UrlName,
		//	Tags:          []string{},
		//	ChildrenUris:  []string{},
		//}

		//switch legacyEntry.Format {
		//case "github-flavored-markdown":
		//	entry.ContentType = "text/markdown"
		//}

		addHashTag := func(tag string, hashtags []string) []string {
			hashtag := "#" + strings.ToLower(strings.Replace(tag, " ", "-", -1))
			if !stringInArray(hashtag, hashtags) {
				return append(hashtags, hashtag)
			}
			return hashtags
		}

		hashtags := []string{}

		for _, tag := range legacyEntry.Tags {
			hashtags = addHashTag(tag, hashtags)
		}

		for _, keyword := range legacyEntry.Keywords {
			hashtags = addHashTag(keyword, hashtags)
		}

		for _, hashtag := range hashtags {
			obj := activitypub.Object{
				Type: "Hashtag",
				Name: activitypub.NaturalLanguageValues{
					activitypub.LangRefValue{
						Value: []byte(hashtag),
					},
				},
			}

			entry.Tag = append(entry.Tag, obj)
		}

		entryDstDir := filepath.Join(dstDir, entryIdStr)
		err = os.MkdirAll(entryDstDir, 0755)
		exitOnErr(err)

		entryDstJson, err := jsonld.WithContext(
			jsonld.IRI(activitypub.ActivityBaseURI),
		).Marshal(entry)
		exitOnErr(err)

		entryDstPath := filepath.Join(entryDstDir, "entry.jsonld")
		err = os.WriteFile(entryDstPath, entryDstJson, 0644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

	}
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func stringInArray(s string, a []string) bool {
	for _, item := range a {
		if item == s {
			return true
		}
	}
	return false
}

func printJson(data interface{}) {
	d, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(d))
}
