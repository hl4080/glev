package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/valyala/fasthttp"
)

var res string

func main() {
	var port int
	flag.IntVar(&port, "port", 8080, "server port")
	flag.Parse()
	go log.Printf("http server started on port %d", port)
	err := fasthttp.ListenAndServe(fmt.Sprintf(":%d", port),
		func(c *fasthttp.RequestCtx) {
			_, werr := c.WriteString("Hello World!\r\n")
			if werr != nil {
				log.Fatal(werr)
			}
		})
	if err != nil {
		log.Fatal(err)
	}
}
