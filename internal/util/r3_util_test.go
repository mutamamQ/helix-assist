package util

import "testing"

func TestR3GetContentUTF16(t *testing.T) {
	// LSP column 8 (UTF-16) is after "héllo w" (é = 1 unit, 2 bytes)
	c := GetContent("héllo wörld", 0, 7)
	if c.ContentBefore != "héllo w" {
		t.Errorf("ContentBefore=%q want %q", c.ContentBefore, "héllo w")
	}
	c = GetContent("😀x = 1", 0, 3) // after "😀x"
	if c.ContentBefore != "😀x" {
		t.Errorf("ContentBefore=%q want %q", c.ContentBefore, "😀x")
	}
}
