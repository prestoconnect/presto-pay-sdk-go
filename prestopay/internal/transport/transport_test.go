package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSend_SuccessNoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := Send(context.Background(), srv.Client(), http.MethodPost, srv.URL, []byte(`{}`), nil, RetryPolicy{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.StatusCode != http.StatusOK || string(res.Body) != "ok" || res.Attempts != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server called %d times, want 1", got)
	}
}

func TestSend_WriteOpDoesNotRetryOn500(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	policy := RetryPolicy{MaxRetries: 3, InitialBackoff: time.Millisecond, RetryServerErrors: false}
	res, err := Send(context.Background(), srv.Client(), http.MethodPost, srv.URL, []byte(`{}`), nil, policy)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.StatusCode != http.StatusInternalServerError || res.Attempts != 1 {
		t.Fatalf("a write operation must not auto-retry a 500: %+v", res)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server called %d times, want 1 (no retry for a write op)", got)
	}
}

func TestSend_ReadOpRetriesOn500ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	policy := RetryPolicy{MaxRetries: 3, InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond, RetryServerErrors: true}
	res, err := Send(context.Background(), srv.Client(), http.MethodPost, srv.URL, []byte(`{}`), nil, policy)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.StatusCode != http.StatusOK || res.Attempts != 3 {
		t.Fatalf("expected success on the 3rd attempt: %+v", res)
	}
}

func TestSend_ReadOpGivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	policy := RetryPolicy{MaxRetries: 2, InitialBackoff: time.Millisecond, RetryServerErrors: true}
	res, err := Send(context.Background(), srv.Client(), http.MethodPost, srv.URL, []byte(`{}`), nil, policy)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Attempts != 3 { // the original attempt plus 2 retries
		t.Fatalf("Attempts = %d, want 3", res.Attempts)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("server called %d times, want 3", got)
	}
}

func TestSend_HonoursRetryAfterSeconds(t *testing.T) {
	var calls int32
	var firstCallAt, secondCallAt time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			firstCallAt = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		secondCallAt = time.Now()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	policy := RetryPolicy{MaxRetries: 1, InitialBackoff: time.Microsecond, RetryServerErrors: true}
	res, err := Send(context.Background(), srv.Client(), http.MethodPost, srv.URL, []byte(`{}`), nil, policy)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got := secondCallAt.Sub(firstCallAt); got < 900*time.Millisecond {
		t.Fatalf("retry happened after %v, want at least ~1s per Retry-After", got)
	}
}

func TestSend_ContextDeadlineDuringBackoffStopsRetrying(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	policy := RetryPolicy{MaxRetries: 100, InitialBackoff: 200 * time.Millisecond, RetryServerErrors: true}
	_, err := Send(ctx, srv.Client(), http.MethodPost, srv.URL, []byte(`{}`), nil, policy)
	if err == nil {
		t.Fatal("expected the context deadline to cut retries short")
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	if got, want := parseRetryAfter("5"), 5*time.Second; got != want {
		t.Fatalf("parseRetryAfter(\"5\") = %v, want %v", got, want)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(2 * time.Second).UTC()
	got := parseRetryAfter(future.Format(http.TimeFormat))
	if got <= 0 || got > 3*time.Second {
		t.Fatalf("parseRetryAfter(HTTP-date) = %v, want roughly 2s", got)
	}
}

func TestParseRetryAfter_Garbage(t *testing.T) {
	if got := parseRetryAfter("not a valid value"); got != 0 {
		t.Fatalf("parseRetryAfter(garbage) = %v, want 0", got)
	}
}

func TestBackoffDelay_CapsAtMaxBackoff(t *testing.T) {
	policy := RetryPolicy{InitialBackoff: time.Second, MaxBackoff: 2 * time.Second}
	for attempt := 0; attempt < 10; attempt++ {
		if d := backoffDelay(policy, attempt); d > policy.MaxBackoff {
			t.Fatalf("backoffDelay(attempt=%d) = %v, exceeds MaxBackoff %v", attempt, d, policy.MaxBackoff)
		}
	}
}

func TestBackoffDelay_ZeroInitialBackoffMeansNoDelay(t *testing.T) {
	if d := backoffDelay(RetryPolicy{}, 0); d != 0 {
		t.Fatalf("backoffDelay with zero InitialBackoff = %v, want 0", d)
	}
}
