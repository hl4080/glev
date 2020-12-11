// +build netbsd freebsd dragonfly linux

package util

import (
	"syscall"
)

func SetKeepAlive(fd, secs int) error {
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1); err != nil {
		return err

	}
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, secs); err != nil {
		return err

	}
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs)

}