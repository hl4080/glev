package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/hl4080/glev"
	"github.com/hl4080/glev/internal/log"
)

func main() {
	var port int
	var loops int
	var udp bool
	var trace bool
	var reuseport bool
	var stdlib bool

	flag.IntVar(&port, "port", 5000, "server port")
	flag.BoolVar(&udp, "udp", false, "listen on udp")
	flag.BoolVar(&reuseport, "reuseport", false, "reuseport (SO_REUSEPORT)")
	flag.BoolVar(&trace, "trace", false, "print packets to console")
	flag.IntVar(&loops, "loops", 0, "num loops")
	flag.BoolVar(&stdlib, "stdlib", false, "use stdlib")
	flag.Parse()

	var events glev.EventHandler
	events.NumLoops = loops
	events.OnServing = func(srv glev.Server) (action glev.Action) {
		log.Logger.Printf("echo server started on port %d (loops: %d)", port, srv.NumLoops)
		if reuseport {
			log.Logger.Printf("reuseport")
		}
		if stdlib {
			log.Logger.Printf("stdlib")
		}
		return
	}
	events.Reactor = func(c glev.Conn, in []byte) (out []byte, action glev.Action) {
		if trace {
			log.Logger.Printf("%s", strings.TrimSpace(string(in)))
		}
		out = in
		return
	}
	scheme := "tcp"
	if udp {
		scheme = "udp"
	}
	if stdlib {
		scheme += "-net"
	}
	log.Logger.Fatal(glev.Serve(events, fmt.Sprintf("%s://:%d?reuseport=%t", scheme, port, reuseport)))
}
