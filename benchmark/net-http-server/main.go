package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/hl4080/glev/internal/log"
)

var res string

func main() {
	var port int
	var aaaa bool
	flag.IntVar(&port, "port", 8080, "server port")
	flag.BoolVar(&aaaa, "aaaa", false, "aaaaa....")
	flag.Parse()
	if aaaa {
		res = strings.Repeat("a", 1024)
	} else {
		res = "Hello World!\r\n"
	}
	log.Logger.Printf("http server started on port %d", port)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(res))
	})
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Logger.Fatal(err)
	}
}
