package transport

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This file drives real sockets through each way a request can fail,
// proving that the httptrace-derived RequestNotSent signal agrees with
// Classify's error-type inspection. A stdlib change that reorders when
// WroteHeaderField/WroteRequest fire would fail these tests instead of
// silently changing retry and idempotency behavior.

func attemptWithTimeout(t *testing.T, url string, timeout time.Duration) (bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, requestNotSent, err := Attempt(ctx, client, http.MethodPost, url, []byte(`{}`), nil)
	if resp != nil {
		resp.Body.Close()
	}
	return requestNotSent, err
}

func TestSocket_UnresolvableHost(t *testing.T) {
	requestNotSent, err := attemptWithTimeout(t, "https://this-host-does-not-exist.invalid.", 5*time.Second)
	if err == nil {
		t.Fatal("expected an error for an unresolvable host")
	}
	if !requestNotSent {
		t.Fatal("RequestNotSent should be true: DNS resolution never got a connection to write on")
	}
	if !Classify(err) {
		t.Errorf("Classify disagrees with the httptrace signal for a DNS failure: %v", err)
	}
}

func TestSocket_ConnectionRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := l.Addr().String()
	l.Close() // nothing listens here now

	requestNotSent, err := attemptWithTimeout(t, "http://"+addr+"/", 5*time.Second)
	if err == nil {
		t.Fatal("expected a connection-refused error")
	}
	if !requestNotSent {
		t.Fatal("RequestNotSent should be true: a refused connection never wrote anything")
	}
	if !Classify(err) {
		t.Errorf("Classify disagrees with the httptrace signal for connection refused: %v", err)
	}
}

func TestSocket_SelfSignedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// A default client trusts no certificate the test server presents.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := &http.Client{}
	resp, requestNotSent, err := Attempt(ctx, client, http.MethodPost, srv.URL, []byte(`{}`), nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected a certificate verification error")
	}
	if !requestNotSent {
		t.Fatal("RequestNotSent should be true: the TLS handshake failed before any HTTP bytes were sent")
	}
	if !Classify(err) {
		t.Errorf("Classify disagrees with the httptrace signal for a certificate failure: %v", err)
	}
}

func TestSocket_BlackholedConnect(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	// Deliberately never call Accept: the TCP handshake still completes at
	// the kernel's backlog queue, so the client's dial succeeds and it can
	// write the request into the (unread) receive buffer, but no response
	// ever comes.

	requestNotSent, err := attemptWithTimeout(t, "http://"+l.Addr().String()+"/", 1*time.Second)
	if err == nil {
		t.Fatal("expected a context deadline error waiting for a response that never comes")
	}
	if requestNotSent {
		t.Fatal("RequestNotSent should be false: the request was written into the accepted connection's buffer")
	}
}

func TestSocket_ResetAfterBodyWritten(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	accepted := make(chan struct{})
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Confirm the request actually arrived before resetting.
		_, _ = bufio.NewReader(conn).ReadString('\n')
		close(accepted)
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetLinger(0) // force RST instead of a graceful FIN
		}
	}()

	requestNotSent, err := attemptWithTimeout(t, "http://"+l.Addr().String()+"/", 5*time.Second)
	<-accepted
	if err == nil {
		t.Fatal("expected a connection-reset error")
	}
	if requestNotSent {
		t.Fatal("RequestNotSent should be false: the server read the request before resetting the connection")
	}
}

func TestSocket_HeadersNeverSent_ContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before Attempt ever dials

	client := &http.Client{}
	resp, requestNotSent, err := Attempt(ctx, client, http.MethodPost, "http://127.0.0.1:1/", []byte(`{}`), nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected a context-cancelled error")
	}
	if !requestNotSent {
		t.Fatal("RequestNotSent should be true: the request was cancelled before it could be sent at all")
	}
}
