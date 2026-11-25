package netcheck

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsBindAddrBusy(t *testing.T) {
	if !isBindAddrBusy(errors.New("bind: address already in use")) {
		t.Fatal("unix message")
	}
	if !isBindAddrBusy(errors.New("bind: Only one usage of each socket address is normally permitted")) {
		t.Fatal("windows message")
	}
	if isBindAddrBusy(errors.New("connection refused")) {
		t.Fatal("refused must not retry ports")
	}
	if isBindAddrBusy(context.DeadlineExceeded) {
		t.Fatal("timeout must not retry ports")
	}
}

func TestBindPortRangeDialer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(204)
	}))
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	first, last := 62000, 62005
	dial := BindPortRangeDialer(first, last)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, err := dial(ctx, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	la, ok := c.LocalAddr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("LocalAddr = %T", c.LocalAddr())
	}
	if la.Port < first || la.Port > last {
		t.Fatalf("port %d not in %d-%d", la.Port, first, last)
	}
}
