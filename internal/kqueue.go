package internal

import (
	"glev/internal/log"
	"syscall"
)

type Poll struct {
	kq 		int					//describer for kqueue
	changes	[]syscall.Kevent_t	//changes list for kqueue
	note 	*LKQueue				//lock free unbound queue
}

//create poll and initilaize kq
func NewPoll() *Poll {
	p := new(Poll)
	p.note = NewLKQueue()
	kq, err := syscall.Kqueue()
	if err != nil {
		log.Logger.Fatal(err)
	}
	p.kq = kq
	_, err = syscall.Kevent(kq, []syscall.Kevent_t{{
		Ident: 	0,
		Filter: syscall.EVFILT_USER,
		Flags: 	syscall.EV_ADD | syscall.EV_CLEAR}}, nil, nil)
	if err != nil {
		log.Logger.Fatal(err)
	}
	return p
}

func (p *Poll) Wait(iter func(fd int, note interface{}) error) error {
	events := make([]syscall.Kevent_t, 128)
	for {
		n, err := syscall.Kevent(p.kq, p.changes, events, nil)
		if err != nil && err != syscall.EINTR {
			return err
		}
		//log.Logger.Print(n)
		p.changes = p.changes[:0]
		if err := p.note.HandleEach(func(n interface{}) error {
			return iter(0, n)
		}); err != nil {
			return err
		}
		for i:=0; i<n; i++ {
			if fd := int(events[i].Ident); fd != 0 {
				if err := iter(fd, nil); err != nil {
					return err
				}
			}
		}
	}
}

func (p *Poll) Trigger(note interface{}) error {
	p.note.Enqueue(note)
	_, err := syscall.Kevent(p.kq, []syscall.Kevent_t{{
		Ident: 	0,
		Filter: syscall.EVFILT_USER,
		Fflags:	syscall.NOTE_TRIGGER}}, nil, nil)
	return err
}

//close kq
func (p *Poll) Close() {
	syscall.Close(p.kq)
}

func (p *Poll) AddRead(fd int) {
	p.changes = append(p.changes, syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_READ,
		Flags:  syscall.EV_ADD,
	})
}

func (p *Poll) AddReadWrite(fd int) {
	p.changes = append(p.changes, syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_READ,
		Flags:  syscall.EV_ADD,
	})
	p.changes = append(p.changes, syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_WRITE,
		Flags:  syscall.EV_ADD,
	})
}

func (p *Poll) DelWrite(fd int) {
	p.changes = append(p.changes, syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_WRITE,
		Flags:  syscall.EV_DELETE,
	})
}

func (p *Poll) DelReadWrite(fd int) {
	p.changes = append(p.changes, syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_WRITE,
		Flags:  syscall.EV_DELETE,
	})
	p.changes = append(p.changes, syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_READ,
		Flags:  syscall.EV_DELETE,
	})
}

