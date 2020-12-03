package glev

import (
	"glev/internal"
	"glev/util"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type conn struct {
	fd         int              //file describer
	lnidx      int              //listener index in server's listener list
	out        []byte           //write buffer
	sa         syscall.Sockaddr //remote address
	reuse      bool             //should reuse input buffer
	opened     bool             //opened event fired
	action     Action           //next user action
	ctx        interface{}      //user-defined context
	addrIdx    int              //index of listening address
	localAddr  net.Addr         //local address
	remoteAddr net.Addr         //remote address
	loop       *loop            //connection loop
}

//conn implementation
func (c *conn) Context() interface{} {return c.ctx}
func (c *conn) SetContext(ctx interface{}) {c.ctx = ctx	}
func (c *conn) LocalAddr() net.Addr {return c.localAddr}
func (c *conn) RemoteAddr() net.Addr {return c.remoteAddr}
func (c *conn) Wake() {
	if c.loop != nil {
		c.loop.poll.Trigger(c)
	}
}

type server struct{
	eventHandler EventHandler       //all the events
	loops        []*loop            //all the loops
	lns          []*listener        //all the listeners
	wg           sync.WaitGroup     //wait group for loops
	cond         *sync.Cond          //shutdown condition
	balance      LoadBalance        //load balance strategy
	accepted     uintptr            //accept counter
	tch          chan time.Duration //tick interval
}

type loop struct {
	idx     int            // loop index in the server loops list
	poll    *internal.Poll // epoll or kqueue
	packet  []byte         // read packet buffer
	fdconns map[int]*conn  // loop connections fd -> conn
	count   int32          // connection count
}

//wait for a signal to shut down the server
func (s *server) waitShutdown() {
	s.cond.L.Lock()
	s.cond.Wait()
	s.cond.L.Unlock()
}

//send a signal to shut down the server
func (s *server) signalShutdown() {
	s.cond.L.Lock()
	s.cond.Signal()
	s.cond.L.Unlock()
}

func serve(eventHander EventHandler, listeners []*listener) error {
	numLoops := eventHander.NumLoops
	if numLoops == 0 {
		numLoops = 1
	}
	if numLoops < 0 {
		numLoops = runtime.NumCPU()
	}
	s := &server{
		eventHandler:	eventHander,
		lns:			listeners,
		cond:			sync.NewCond(&sync.Mutex{}),
		balance: 		eventHander.LoadBalance,
		tch:			make(chan time.Duration),
	}

	//serving is ready for connection, and starting information has been provided
	if s.eventHandler.OnServing != nil {
		var svr Server
		svr.NumLoops = numLoops
		svr.Addrs = make([]net.Addr, len(listeners))
		for i, ln := range listeners {
			svr.Addrs[i] = ln.lnaddr
		}
		action := s.eventHandler.OnServing(svr)
		switch action {
		case None:
		case Shutdown:
			return nil
		}

		defer func() {
			//wait for signal to shutdown the server
			s.waitShutdown()
			//note all the loops to close
			for _, l := range s.loops {
				l.poll.Trigger(errCloseServer)
			}
			//wait loops to complete the read events
			s.wg.Wait()
			//close loops and associated connections
			for _, l := range s.loops {
				for _, con := range l.fdconns {
					loopCloseConn(s, l, con, nil)
				}
				l.poll.Close()
			}

		}()

		//create loops and bind listeners
		for i:=0; i<numLoops; i++ {
			l := &loop{
				idx:     i,
				poll:    internal.NewPoll(),
				packet:  make([]byte, 0xFFFF),
				fdconns: make(map[int]*conn),
			}
			for _, ln := range listeners {
				l.poll.AddRead(ln.fd)
			}
			s.loops = append(s.loops, l)
		}
		s.wg.Add(len(s.loops))
	}
	for _, l := range s.loops {
		go loopRun(s, l)
	}
	return nil
}

func loopRun(s *server, l *loop) {
	defer func() {
		s.signalShutdown()
		s.wg.Done()
	}()
	if l.idx == 0 && s.eventHandler.Tick != nil {
		go loopTicker(s, l)
	}
	l.poll.Wait(func(fd int, note interface{}) error {
		if fd == 0 {
			return loopNote(s, l, note)
		}
		c := l.fdconns[fd]
		switch {
		case c == nil:
			//log.Logger.Printf("here 1")
			return loopAccept(s, l, fd)
		case !c.opened:
			//log.Logger.Printf("here 2")
			return loopOpened(s, l, c)
		case len(c.out)>0:
			//log.Logger.Printf("here 3")
			return loopWrite(s, l, c)
		case c.action != None:
			//log.Logger.Printf("here 4")
			return loopAction(s, l, c)
		default:
			//log.Logger.Printf("here 5")
			return loopRead(s, l, c)
		}
	})
}

func loopRead(s *server, l *loop, c *conn) error {
	var in []byte
	n, err := syscall.Read(c.fd, l.packet)
	if n == 0 || err != nil {
		if err == syscall.EINTR {
			return nil
		}
		return loopCloseConn(s, l, c, err)
	}
	in = l.packet[:n]
	if !c.reuse {
		in = append([]byte{}, in...)
	}
	if s.eventHandler.Reactor != nil {
		out, action := s.eventHandler.Reactor(c, in)
		c.action = action
		if len(out) > 0 {
			c.out = append([]byte{}, out...)
		}
	}
	if len(c.out) != 0 || c.action == None {
		l.poll.AddReadWrite(c.fd)
	}
	return nil
}

func loopAction(s *server, l *loop, c *conn) error {
	switch c.action {
	case Close:
		return loopCloseConn(s, l, c, nil)
	case Shutdown:
		return errCloseServer
	case Detach:
		return loopDetachConn(s, l, c, nil)
	default:
		c.action = None
	}
	if len(c.out) == 0 && c.action == None {
		l.poll.DelWrite(c.fd)
	}
	return nil
}

func loopWrite(s *server, l *loop, c *conn) error {
	if s.eventHandler.PreWrite != nil {
		s.eventHandler.PreWrite()
	}
	n, err := syscall.Write(c.fd, c.out)
	if err != nil {
		if err != syscall.EAGAIN {
			return nil
		}
		return loopCloseConn(s, l, c, err)
	}
	if n == len(c.out) {
		//if c.out oversize a page, then release the page, otherwise reuse the existed c.out
		if cap(c.out) > 4096 {
			c.out = nil
		} else {
			c.out = c.out[:0]
		}
	} else {
		c.out = c.out[n:]
	}
	if len(c.out) == 0 && c.action == None {
		l.poll.DelWrite(c.fd)
	}
	return nil
}

func loopDetachConn(s *server, l *loop, c *conn, err error) error {
	if s.eventHandler.OnDetached == nil {
		s.eventHandler.OnClosed(c, err)
	}
	l.poll.DelReadWrite(c.fd)
	atomic.AddInt32(&l.count, -1)
	delete(l.fdconns, c.fd)
	if err := syscall.SetNonblock(c.fd, false); err != nil {
		return err
	}
	switch s.eventHandler.OnDetached(c, &detachedConn{c.fd}) {
	case None:
	case Shutdown:
		return errCloseServer
	}
	return err
}

func loopCloseConn(s *server, l *loop, c *conn, err error) error {
	atomic.AddInt32(&l.count, -1)
	delete(l.fdconns, c.fd)
	syscall.Close(c.fd)
	if s.eventHandler.OnClosed != nil {
		action := s.eventHandler.OnClosed(c, err)
		switch action {
		case Shutdown:
			return errCloseServer
		case None:
		}
	}
	return nil
}

func loopOpened(s *server, l *loop, c *conn) error {
	c.opened = true
	c.addrIdx = c.lnidx
	c.localAddr = s.lns[c.lnidx].lnaddr
	c.remoteAddr = util.Sockaddr2Addr(c.sa)
	if s.eventHandler.OnOpened != nil {
		out, ops, action := s.eventHandler.OnOpened(c)
		if len(out) > 0 {
			c.out = append([]byte{}, out...)
		}
		c.action = action
		c.reuse = ops.ReuseInputBuffer
		if ops.TCPKeepAlive > 0 {
			if _, ok := s.lns[c.lnidx].ln.(*net.TCPListener); ok {
				util.SetKeepAlive(c.fd, int(ops.TCPKeepAlive/time.Second))
			}
		}
	}
	if len(c.out) == 0 && c.action == None {
		l.poll.DelWrite(c.fd)
	}
	return nil
}

func loopAccept(s *server, l *loop, fd int) error {
	for i, ln := range s.lns {
		if ln.fd == fd {
			//make sure this is the right loop to accept this fd
			if len(s.loops) > 1 {
				switch s.balance {
				case RoundRobin:
					idx := int(atomic.LoadUintptr(&s.accepted)) % len(s.loops)
					if idx != l.idx {
						return nil	//do not accept for loop index does not match the RoundRobin rule
					}
					atomic.AddUintptr(&s.accepted, 1)
				case LeastConnections:
					n := atomic.LoadInt32(&l.count)
					for i:=0; i<len(s.loops); i++ {
						if atomic.LoadInt32(&s.loops[i].count) < n {
							return nil	//do not accept because other loop has less connections
						}
					}
					atomic.AddInt32(&l.count, 1)
				}
			}
			if ln.pconn != nil {
				return loopUdpRead(s, l, i, fd)
			}
			newfd, sa, err := syscall.Accept(fd)
			if err != nil {
				if err == syscall.EAGAIN {
					return nil
				}
				return err
			}
			if err := syscall.SetNonblock(newfd, true); err != nil {
				return err
			}
			c := &conn{
				fd:         newfd,
				lnidx:      i,
				out:        nil,
				sa:         sa,
				loop:       l,
			}
			l.fdconns[c.fd] = c
			l.poll.AddReadWrite(c.fd)
			atomic.AddInt32(&l.count, 1)
			break
		}
	}
	return nil
}

func loopUdpRead(s *server, l *loop, lnidx, fd int) error {
	n, sa, err := syscall.Recvfrom(fd, l.packet, 0)
	if err != nil || n == 0 {
		return nil
	}
	if s.eventHandler.Reactor != nil {
		c := &conn{}
		c.remoteAddr = util.Sockaddr2Addr(sa)
		c.localAddr = s.lns[lnidx].lnaddr
		c.lnidx = lnidx
		in := append([]byte{}, l.packet[:n]...)
		out, action := s.eventHandler.Reactor(c, in)
		if len(out) > 0 {
			if s.eventHandler.PreWrite != nil {
				s.eventHandler.PreWrite()
			}
			syscall.Sendto(fd, out, 0, sa)
		}
		switch action {
		case Shutdown:
			return errCloseServer
		}
	}
	return nil
}

func loopNote(s *server, l *loop, note interface{}) error {
	var err error
	switch v := note.(type) {
	case error:
		err = v
	case time.Duration:
		delay, action := s.eventHandler.Tick()
		switch action {
		case None:
		case Shutdown:
			err = errCloseServer
		}
		s.tch <- delay
	case *conn:
		if l.fdconns[v.fd] != v {
			return nil	//connection has already existed in loop, ignore it
		}
		return loopWake(s, l ,v)
	}
	return err
}

func loopWake(s *server, l *loop, c *conn) error {
	if s.eventHandler.Reactor == nil {
		return nil
	}
	out, action := s.eventHandler.Reactor(c, nil)
	c.action = action
	if len(out) >0 {
		c.out = append([]byte{}, out...)
	}
	if len(c.out) != 0 && c.action != None {
		l.poll.AddReadWrite(c.fd)
	}
	return nil
}

func loopTicker(s *server, l *loop) {
	for {
		if err := l.poll.Trigger(time.Duration(0)); err != nil {
			break
		}
		time.Sleep(<-s.tch)
	}
}

type detachedConn struct {
	fd int
}

func (dc *detachedConn) Read(p []byte) (int, error) {
	n, err := syscall.Read(dc.fd, p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		if len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	return n, nil
}

func (dc *detachedConn) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		nn, err := syscall.Write(dc.fd, p)
		if err != nil {
			return n, err
		}
		p = p[nn:]
	}
	return n, nil
}

func (dc *detachedConn) Close() error {
	err := syscall.Close(dc.fd)
	if err != nil {
		return err
	}
	dc.fd = -1
	return nil
}

func (ln *listener) close() {
	if ln.ln != nil {
		ln.ln.Close()
	}
	if ln.pconn != nil {
		ln.pconn.Close()
	}
	if ln.f != nil {
		ln.f.Close()
	}
	if ln.fd != 0 {
		syscall.Close(ln.fd)
	}
	if ln.network == "unix" {
		os.RemoveAll(ln.addr)
	}
}

func (ln *listener) system() error {
	var err error
	switch lnnet := ln.ln.(type) {
	case *net.TCPListener:
		ln.f ,err = lnnet.File()
	case *net.UnixListener:
		ln.f, err = lnnet.File()
	case nil:
		switch pconn := ln.pconn.(type) {
		case *net.UDPConn:
			ln.f, err = pconn.File()
		}
	}
	if err != nil {
		ln.close()
		return err
	}
	ln.fd = int(ln.f.Fd())
	return syscall.SetNonblock(ln.fd, true)
}