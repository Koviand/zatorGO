package netcheck

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

// BindPortRangeDialer возвращает DialContext, который привязывает исходящие
// TCP-соединения к локальным портам [first, last] (round-robin). Нужен для
// изолированного SuperSweep: трафик проб попадает под mark по sport.
// Повтор на следующем порту — только при EADDRINUSE / «address already in use»;
// timeout/refused/unreachable возвращаются сразу (без шторма SYN/DNS).
func BindPortRangeDialer(first, last int) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if first <= 0 || last < first || last > 65535 {
		panic(fmt.Sprintf("netcheck: некорректный диапазон портов %d-%d", first, last))
	}
	var seq atomic.Uint32
	n := last - first + 1
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		timeout := DefaultConnectTimeout
		if dl, ok := ctx.Deadline(); ok {
			if rem := time.Until(dl); rem > 0 && rem < timeout {
				timeout = rem
			}
		}
		d := net.Dialer{Timeout: timeout}
		start := int(seq.Add(1)-1) % n
		var lastErr error
		for i := 0; i < n; i++ {
			port := first + (start+i)%n
			d.LocalAddr = &net.TCPAddr{Port: port}
			c, err := d.DialContext(ctx, network, addr)
			if err == nil {
				return c, nil
			}
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !isBindAddrBusy(err) {
				return nil, err
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("netcheck: нет свободного порта в %d-%d", first, last)
		}
		return nil, lastErr
	}
}

// isBindAddrBusy — локальный порт занят; имеет смысл взять следующий sport.
// Строковое сравнение: portable (unix EADDRINUSE + Windows WSAEADDRINUSE).
func isBindAddrBusy(err error) bool {
	if err == nil {
		return false
	}
	if errnoAddrInUse(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}
