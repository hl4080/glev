package glev

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"
)

var errCloseServer  = errors.New("close server!")
var errCloseConn 	= errors.New("close conn!")

//Action after completion of an event
type Action int

const (
	//None indicates no action should be applied after an event completed
	None Action = iota
	//Close the connection
	Close
	//Shutdown the server
	Shutdown
)

//LoadBalance sets the load balance method
type LoadBalance int

const (
	//Random means that connections are randomly distributed
	Random LoadBalance = iota
	//Round Robin means connections are distributed to a loop in round-robin fashion
	RoundRobin
	//LeastConnections means new connection is allocated to a loop with least connections hold
	LeastConnections
)

// Options are set when the client opens.
type Options struct {
	// TCPKeepAlive (SO_KEEPALIVE) socket option.
	TCPKeepAlive time.Duration
	// ReuseInputBuffer will forces the connection to share and reuse the
	// same input packet buffer with all other connections that also use
	// this option.
	// Default value is false, which means that all input data which is
	// passed to the Data event will be a uniquely copied []byte slice.
	ReuseInputBuffer bool
}

// Server represents a server context which provides information about the
// running server and has control functions for managing state.
type Server struct {
	Addrs 			[]net.Addr		//listening addr for server
	NumLoops 		int				//number of eventloops for server
}

//Conn is a glev connection
type Conn interface {
	//Context returns a user-defined context
	Context() interface{}
	//SetContext sets a user-defined context
	SetContext(interface{})
	//LocalAddr is the connection's local socket address
	LocalAddr() net.Addr
	//RemoteAddr is the connection's remote peer address
	RemoteAddr() net.Addr
	//Wake triggers a data event for this connection
	Wake()
}

//EventHandler represents the event's callback for the server call
//each event has an action return value that is used to manage the state of the connection and server
type EventHandler struct {
	// NumLoops sets the number of loops to use for the server. Setting this
	// to a value greater than 1 will effectively make the server
	// multithreaded for multi-core machines. Which means you must take care
	// with synchonizing memory between all event callbacks. Setting to 0 or 1
	// will run the server single-threaded. Setting to -1 will automatically
	// assign this value equal to runtime.NumProcs().
	NumLoops int
	// LoadBalance sets the load balancing method. Load balancing is always a
	// best effort to attempt to distribute the incoming connections between
	// multiple loops. This option is only works when NumLoops is set.
	LoadBalance LoadBalance
	//OnServing fires when server is ready for connections
	//parameter of server has information and utilities
	OnServing 	func(server Server) (action Action)
	//OnOpen fires when a new connection is opened
	//parameter of conn contains information about this connection such as address and context
	//return para of out indicates the information server sends back to client
	OnOpened 	func(conn Conn) (out []byte, ops Options, action Action)
	//OnClosed fired when a connection has closed
	//parameter of err indicates the connection error for closing
	OnClosed 	func(conn Conn, err error) (action Action)
	//PreWrite fires before any data is written to client socket
	//usually used to put some prepositive operations and logging information before writting
	PreWrite	func()
	//Reactor fires when a connection sends data to server
	//parameter of in indicates the incoming data from client
	//out is used to send data back to client
	Reactor		func(conn Conn, in []byte) (out []byte, action Action)
	//Tick fires immediately after the server start and fires every certain duration of time interval
	Tick		func() (delay time.Duration, action Action)
}

type listener struct {
	ln			net.Listener
	lnaddr		net.Addr
	pconn 		net.PacketConn
	reuseport	bool
	f			*os.File
	fd 			int
	network 	string
	addr 		string
}

// Serve starts handling events for the specified addresses.
//
// Addresses should use a scheme prefix and be formatted
// like `tcp://192.168.0.10:9851` or `unix://socket`.
// Valid network schemes:
//  tcp   - bind to both IPv4 and IPv6
//  tcp4  - IPv4
//  tcp6  - IPv6
//  udp   - bind to both IPv4 and IPv6
//  udp4  - IPv4z
//  udp6  - IPv6
//  unix  - Unix Domain Socket
//
// The "tcp" network scheme is assumed when one is not specified.
func Serve(eventHandler EventHandler, addr ...string) error {
	var lns []*listener
	defer func() {
		for _, ln := range lns {
			ln.close()
		}
	}()
	var stdlib bool
	for _, addr := range addr {
		var ln listener
		var stdlibt bool
		ln.network, ln.addr, ln.reuseport, stdlibt = parseAddr(addr)
		if stdlibt {
			stdlib = true
		}
		if ln.network == "unix" {
			os.RemoveAll(ln.addr)	//remove existed socket file for sockets' communication
		}
		var err error
		if ln.network == "udp" {
			if ln.reuseport {
				//ln.pconn, err = reuse
			} else {
				ln.pconn, err = net.ListenPacket(ln.network, ln.addr)
			}
		} else {
			if ln.reuseport {
				//operation for reuseport
			} else {
				ln.ln, err = net.Listen(ln.network, ln.addr)
			}
		}
		if err != nil {
			return err
		}
		if ln.pconn != nil {
			ln.lnaddr = ln.pconn.LocalAddr()
		} else {
			ln.lnaddr = ln.ln.Addr()
		}
		if !stdlib {
			if err := ln.system(); err != nil {
				return err
			}
		}
		lns = append(lns, &ln)
	}
	return serve(eventHandler, lns)
}

// InputStream is a helper type for managing input streams from inside
// the Data event.
type InputStream struct{ b []byte }

// Begin accepts a new packet and returns a working sequence of
// unprocessed bytes.
func (is *InputStream) Begin(packet []byte) (data []byte) {
	data = packet
	if len(is.b) > 0 {
		is.b = append(is.b, data...)
		data = is.b
	}
	return data
}

// End shifts the stream to match the unprocessed data.
func (is *InputStream) End(data []byte) {
	if len(data) > 0 {
		if len(data) != len(is.b) {
			is.b = append(is.b[:0], data...)
		}
	} else if len(is.b) > 0 {
		is.b = is.b[:0]
	}
}

func parseAddr(addr string) (network, address string, reuseport bool, stdlib bool) {
	network = "tcp"
	address = addr
	reuseport = false
	if strings.Contains(address, "://") {
		network = strings.Split(address, "://")[0]
		address = strings.Split(address, "://")[1]
	}
	if strings.HasSuffix(network, "-net") {
		stdlib = true
		network = network[:len(network)-4]
	}
	q := strings.Index(address, "?")
	if q != -1 {
		for _, part := range strings.Split(address[q+1:], "&") {
			kv := strings.Split(part, "=")
			if len(kv) == 2 {
				switch kv[0] {
				case "reuseport":
					if len(kv[1]) != 0 {
						switch kv[1][0] {
						default:
							reuseport = kv[1][0] >= '1' && kv[1][0] <= '9'
						case 'T', 't', 'Y', 'y':
							reuseport = true
						}
					}
				}
			}
		}
		address = address[:q]
	}
	return
}


