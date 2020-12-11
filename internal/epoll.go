// +build linux

package internal

import (
	"syscall"
	"unsafe"
)

type Poll struct {
	fd   int      //epoll fd
	wfd  int      //wake fd
	note *LKQueue //lock free queue
}

//create poll and initialize
func NewPoll() *Poll {
	p := new(Poll)
	fd, err := syscall.EpollCreate1(0)
	if err != nil {
		log.Logger.Fatal(err)
	}
	p.fd = fd
	wfd, _, err0 := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0, 0)
	if err0 != 0 {
		syscall.Close(fd)
		log.Logger.Fatal(err)
	}
	p.note = NewLKQueue()
	p.wfd = int(wfd)
	p.AddRead(p.wfd)
	return p
}

func (p *Poll) Close() error {
	if err := syscall.Close(p.wfd); err != nil {
		return err
	}
	return syscall.Close(p.fd)
}

func (p *Poll) Wait(iter func(fd int, note interface{}) error) error {
	events := make([]syscall.EpollEvent, 128)
	for {
		n, err := syscall.EpollWait(p.fd, events, 100)
		if err != nil && err != syscall.EINTR {
			return err
		}
		if err := p.note.HandleEach(func(note interface{}) error {
			return iter(0, note)
		}); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if fd := int(events[i].Fd); fd != p.wfd {
				if err := iter(fd, nil); err != nil {
					return err
				}
			} else if fd == p.wfd {
				var data [8]byte
				syscall.Read(p.wfd, data[:])
			}
		}
	}
}

func (p *Poll) AddRead(fd int) {
	err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN,
		Fd:     int32(fd),
	})
	if err != nil {
		log.Logger.Fatal(err)
	}
}

func (p *Poll) Trigger(note interface{}) error {
	p.note.Enqueue(note)
	var x int64 = 1
	_, err := syscall.Write(p.wfd, (*(*[8]byte)(unsafe.Pointer(&x)))[:])
	return err
}

func (p *Poll) AddReadWrite(fd int) {
	err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN | syscall.EPOLLOUT,
		Fd:     int32(fd),
	})
	if err != nil {
		log.Logger.Info("here 0")
		log.Logger.Fatal(err)
	}
}

func (p *Poll) Mod2ReadWrite(fd int) {
	err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN | syscall.EPOLLOUT,
		Fd:     int32(fd),
	})
	if err != nil {
		log.Logger.Info("here 7")
		log.Logger.Fatal(err)
	}
}

func (p *Poll) DelWrite(fd int) {
	err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN,
		Fd:     int32(fd),
	})
	if err != nil {
		log.Logger.Fatal("here 1")
		log.Logger.Fatal(err)
	}
}

func (p *Poll) DelReadWrite(fd int) {
	err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_DEL, fd, &syscall.EpollEvent{
		Events: syscall.EPOLLIN | syscall.EPOLLOUT,
		Fd:     int32(fd),
	})
	if err != nil {
		log.Logger.Fatal("here 2")
		log.Logger.Fatal(err)
	}
}
