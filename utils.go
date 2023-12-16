package syndicat

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/go-ap/jsonld"
)

func ensureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

func writeFile(filePath string, data []byte) error {
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func ensureDirWriteFile(filePath string, data []byte) error {
	err := ensureDir(filepath.Dir(filePath))
	if err != nil {
		return err
	}

	return writeFile(filePath, data)
}

func printJson(data interface{}) {
	d, _ := json.MarshalIndent(data, "", "  ")
	//fmt.Println(string(d))
	err := quick.Highlight(os.Stdout, string(d)+"\n", "json", "terminal256", "monokai")
	if err != nil {
		fmt.Println(err.Error())
	}
}

func printJsonLd(data interface{}) {
	d, _ := jsonld.Marshal(data)
	fmt.Println(string(d))
}

func getHost(r *http.Request) string {
	// TODO: check to make sure we're behind a proxy before
	// trusting XFH header
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	return host
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
