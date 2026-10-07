package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leona/helix-assist/internal/lsp"
)

func TestStripSentinel(t *testing.T) {
	for in, want := range map[string]string{
		"@@@\n        x = 1": "        x = 1",
		"@@@\r\n  y":         "  y",
		"no sentinel":        "no sentinel",
		"\n@@@\nz":           "z",
	} {
		if got := stripSentinel(in); got != want {
			t.Errorf("stripSentinel(%q)=%q want %q", in, got, want)
		}
	}
}

func newTestProvider(h http.HandlerFunc, timeoutMs int) (*BryantProvider, func()) {
	srv := httptest.NewServer(h)
	return NewBryantProvider("k", "m", "", srv.URL, timeoutMs, lsp.NewLogger("")), srv.Close
}

func TestRawFinishReason(t *testing.T) {
	for fr, want := range map[string]string{"length": "truncated", "content_filter": "content_filter", "stop": ""} {
		p, done := newTestProvider(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"choices":[{"message":{"content":"@@@\nhi"},"finish_reason":%q}]}`, fr)
		}, 1000)
		out, err := p.Raw(context.Background(), "", "s", "u", 10)
		done()
		if want == "" {
			if err != nil || out != "hi" {
				t.Errorf("stop: %q %v", out, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err=%v", fr, err)
		}
	}
}

func TestStatusErrors(t *testing.T) {
	p, done := newTestProvider(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"expired"}`))
	}, 1000)
	_, err := p.Raw(context.Background(), "", "s", "u", 10)
	done()
	if err == nil || err.Error() != "bridge auth failed (401): run bryant-provider refresh" {
		t.Errorf("401: %v", err)
	}
	p, done = newTestProvider(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(strings.Repeat("x", 1000)))
	}, 1000)
	_, err = p.Raw(context.Background(), "", "s", "u", 10)
	done()
	if err == nil || len(err.Error()) > 260 {
		t.Errorf("500: %v", err)
	}
}

// Provider timeout applies only without a ctx deadline; a longer action
// deadline wins (scaled: 50ms provider timeout, 150ms server delay).
func TestTimeoutPrecedence(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}
	p, done := newTestProvider(slow, 50)
	defer done()
	if _, err := p.Raw(context.Background(), "", "s", "u", 10); err == nil {
		t.Error("no-deadline ctx should hit provider timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.Raw(ctx, "", "s", "u", 10); err != nil {
		t.Errorf("action deadline should override provider timeout: %v", err)
	}
}
