package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leona/helix-assist/internal/lsp"
)

// At 269dc18 chat() always applied p.timeout (fetch-timeout) on top of the
// caller ctx, so Completion was bounded by min(fetch, completion) timeout.
func TestR3CompletionStillHonoursFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-r.Context().Done():
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer srv.Close()
	p := NewBryantProvider("k", "m", "m", srv.URL, 200, lsp.NewLogger("")) // fetch-timeout 200ms
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	_, err := p.Completion(ctx, CompletionRequest{}, "f.go", "go", 1)
	if err == nil || time.Since(start) > 800*time.Millisecond {
		t.Fatalf("fetch-timeout (200ms) no longer bounds completion: err=%v after %v", err, time.Since(start))
	}
}
