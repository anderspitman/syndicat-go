package syndicat

import (
	"fmt"
	"log"
	"net/http"

	"github.com/anderspitman/treemess-go"
	"github.com/gemdrive/gemdrive-go"
	"github.com/lastlogin-io/obligator"
)

type ServerConfig struct {
	RootUri string
}

type Server struct{}

func NewServer(conf ServerConfig) *Server {

	rootUri := conf.RootUri
	authUri := "auth." + rootUri

	authConfig := obligator.ServerConfig{
		RootUri: "https://" + authUri,
	}

	authServer := obligator.NewServer(authConfig)
	fmt.Println(authServer)

	gdConfig := &gemdrive.Config{
		Port: 3839,
	}

	tmess := treemess.NewTreeMess()
	gdTmess := tmess.Branch()

	_, err := gemdrive.NewServer(gdConfig, gdTmess)
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan treemess.Message)
	tmess.Listen(ch)

	tmess.Send("start", nil)

	go func() {
		for msg := range ch {
			fmt.Println(msg)
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		// TODO: check to make sure we're behind a proxy before
		// trusting XFH header
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}

		fmt.Println(host)

		switch host {
		case rootUri:
			fmt.Println("root")
		case authUri:
			fmt.Println("auth")
			authServer.ServeHTTP(w, r)
			return
		}
	})

	http.ListenAndServe(":9005", nil)

	s := &Server{}
	return s
}
