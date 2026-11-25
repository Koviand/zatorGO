//go:build windows

package netcheck

import (
	"errors"
	"syscall"
)

const wsaEAddrInUse = 10048

func errnoAddrInUse(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && uintptr(errno) == wsaEAddrInUse
}
