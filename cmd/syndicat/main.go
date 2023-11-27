package main

import (
	"flag"
	"fmt"

	"github.com/anderspitman/syndicat-go"
)

func main() {
	rootUri := flag.String("root-uri", "", "Root URI")
	flag.Parse()

	config := syndicat.ServerConfig{
		RootUri: *rootUri,
	}
	server := syndicat.NewServer(config)
	fmt.Println(server)
}
