package main

import (
	"flag"
	"fmt"

	"github.com/anderspitman/syndicat-go"
)

func main() {
	rootUri := flag.String("root-uri", "", "Root URI")
	templatesDir := flag.String("templates-dir", "templates", "Templates directory")
	port := flag.Int("port", 9005, "Port")
	flag.Parse()

	config := syndicat.ServerConfig{
		RootUri:      *rootUri,
		TemplatesDir: *templatesDir,
		Port:         *port,
	}
	server := syndicat.NewServer(config)
	fmt.Println(server)
}
