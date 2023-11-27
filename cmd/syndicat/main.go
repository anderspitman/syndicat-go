package main

import (
        "fmt"

        "github.com/anderspitman/syndicat-go"
)

func main() {
        server := syndicat.NewServer()
        fmt.Println(server)
}
