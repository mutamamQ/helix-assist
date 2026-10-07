package lsp

import (
	"sync"
	"testing"
)

func TestBufferStoreRace(t *testing.T) {
	s := NewBufferStore()
	s.Set(&Buffer{URI: "u", Text: "a", Version: 0})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			s.UpdateText("u", i, "text")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			if b, ok := s.Get("u"); ok {
				_ = b.Text + string(rune(b.Version))
			}
			if b, ok := s.GetCurrent(); ok {
				_ = b.Text
			}
		}
	}()
	wg.Wait()
}
