//go:build unix

package netcheck

import (
	"errors"
	"syscall"
)

func errnoAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
