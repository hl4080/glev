# glev
## What is glev
glev is high throughput event-driven network library based on reactor pattern, supporting both `kqueue` and `epoll` syscall rather than calling net package.

## How to build
simply build `glev` using `go get`

`go get -u github.com/hl4080/glev`

## Features
 - Tiny but high performance network library
 - IO complexing reactor pattern
 - Low CPU and memory consumption
 - lock free unbounded note queue 
 - Supporting `kqueue` and `epoll` and tested in Macos/Linux
 - Simply API
 
## Usage
glev supports self-defined event handler and passes through function `Serve`. You just need to add your own logic processing in `events.Reactor` like `echo` server listed below:

```
import (
	"github.com/hl4080/glev"
	"github.com/hl4080/glev/internal/log"
)
var events glev.EventHandler
events.Reactor = func(c glev.Conn, in []byte) (out []byte, action glev.Action) {
 		out = in
 		return
 	}
 log.Logger.Fatal(glev.Serve(events, "tcp://localhost:5000"))
 ```
Then simply connect to the echo server and exchange information with it
```
telnet localhost 5000
```

## EventHandlers
- OnServing: certain server parameters once the server is ready for connections
- OnOpened: certain information sent back to client once the connection is ready
- OnClosed: certain information sent back to client once the connection is closed
- PreWrite: Before client sends informations, server pre-writes some inductive steps
- Reactor: Data processing function when receiving information from clients
- Tick: Processing every certain time interval after server starts

## BenchMarks
We simply test our benchmark of `glev` on Macos(1.4 GHz Intel Core i5). Results are shown below:

- echo server
![echo server](benchmark/pic/echo.jpeg#pic_center)

- http server
![http server](benchmark/pic/fasthttp.jpeg#pic_center)

 
 ## Reference
 [muduo: Event-driven network library for multi-threaded Linux server in C++11](https://github.com/chenshuo/muduo)
    
 
