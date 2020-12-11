package util

import (
	"net"
	"syscall"
)

//convert sockaddr to net addr
func Sockaddr2Addr(sa syscall.Sockaddr) net.Addr{
	var naddr net.Addr
	switch addr := sa.(type) {
	case *syscall.SockaddrInet4:
		naddr = &net.TCPAddr{
			IP:   addr.Addr[0:],
			Port: addr.Port,
		}
	case *syscall.SockaddrInet6:
		var zone string
		if addr.ZoneId != 0 {
			if ifi, err := net.InterfaceByIndex(int(addr.ZoneId)); err == nil {
				zone = ifi.Name
			}
		}
		if zone == "" && addr.ZoneId != 0 {
		}
		naddr = &net.TCPAddr{
			IP:   addr.Addr[0:],
			Port: addr.Port,
			Zone: zone,
		}
	case *syscall.SockaddrUnix:
		naddr = &net.UnixAddr{
			Name: addr.Name,
			Net:  "unix",
		}
	}
	return naddr
}