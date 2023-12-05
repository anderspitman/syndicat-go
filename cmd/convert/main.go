package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/anderspitman/syndicat-go"
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
	authorArg := flag.String("author", "", "Author")
	flag.Parse()

	srcDir := *srcDirArg
	dstDir := *dstDirArg
	author := *authorArg

	dirItems, err := os.ReadDir(srcDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	for _, item := range dirItems {
		entryIdStr := item.Name()

		_, err := strconv.Atoi(entryIdStr)
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

		timestamp := legacyEntry.Timestamp
		if len(timestamp) == 20 {
			timestamp = timestamp + "+00:00"
		} else if len(timestamp) == 10 {
			timestamp = timestamp + "T00:00:00Z+00:00"
		}

		entry := &syndicat.Entry{
			Title:         legacyEntry.Title,
			Author:        author,
			PublishedTime: timestamp,
			ModifiedTime:  timestamp,
			Content:       string(contentBytes),
			VanityPath:    legacyEntry.UrlName,
			Tags:          []string{},
			Children:      []string{},
		}

		switch legacyEntry.Format {
		case "github-flavored-markdown":
			entry.ContentType = "text/markdown"
		}

		for _, tag := range legacyEntry.Tags {
			entry.Tags = append(entry.Tags, tag)
		}

		for _, keyword := range legacyEntry.Keywords {
			entry.Tags = append(entry.Tags, keyword)
		}

		//printJson(legacyEntry)
		//printJson(entry)

		entryDstDir := filepath.Join(dstDir, entryIdStr)
		err = os.MkdirAll(entryDstDir, 0755)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		entryDstJson, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		entryDstPath := filepath.Join(entryDstDir, "index.json")
		err = os.WriteFile(entryDstPath, entryDstJson, 0644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

	}
}

func printJson(data interface{}) {
	d, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(d))
}
