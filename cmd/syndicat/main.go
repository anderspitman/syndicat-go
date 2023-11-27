package main

import (
        "fmt"

        "github.com/anderspitman/syndicat"
)

func main() {
        server := syndicat.NewServer()
        fmt.Println(server)
}
