package lsp

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"
)

// didChange notifications are dispatched in one goroutine each (emit), so
// they can be applied out of order and an older version can win.
func TestR3DidChangeOrdering(t *testing.T) {
	bad := 0
	for round := 0; round < 20; round++ {
		var in bytes.Buffer
		send := func(body string) { fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body) }
		send(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///a","languageId":"go","version":0,"text":"v0"}}}`)
		const N = 300
		for i := 1; i <= N; i++ {
			send(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"file:///a","version":%d},"contentChanges":[{"text":"v%d"}]}}`, i, i))
		}
		svc := NewService(ServerCapabilities{}, NewLogger(""), "t")
		svc.stdin, svc.stdout = &in, io.Discard
		svc.Start()
		time.Sleep(100 * time.Millisecond)
		b, _ := svc.Buffers.Get("file:///a")
		if b.Version != N {
			bad++
			t.Logf("round %d: final buffer version=%d text=%q, want %d", round, b.Version, b.Text, N)
		}
	}
	if bad > 0 {
		t.Fatalf("%d/20 rounds ended with a stale buffer", bad)
	}
}
