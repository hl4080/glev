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

//keep alive the connection
func SetKeepAlive(fd, t int) error {
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, 0x8, 1); err != nil {
		return err
	}
	switch err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 0x101, t); err {
	case nil, syscall.ENOPROTOOPT: // OS X 10.7 and earlier don't support this option
	default:
		return err
	}
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPALIVE, t)
}
