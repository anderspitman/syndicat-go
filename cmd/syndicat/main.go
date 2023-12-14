package main

import (
	"flag"
	"fmt"

	"github.com/anderspitman/syndicat-go"
)

func main() {
	rootUri := flag.String("root-uri", "", "Root URI")
	templatesDir := flag.String("templates-dir", "templates", "Templates directory")
	flag.Parse()

	config := syndicat.ServerConfig{
		RootUri:      *rootUri,
		TemplatesDir: *templatesDir,
	}
	server := syndicat.NewServer(config)
	fmt.Println(server)
}
