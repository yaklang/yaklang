//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package pty

import (
	"testing"

	"golang.org/x/term"
)

func TestResizeChangesTerminalSize(t *testing.T) {
	terminal, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	for _, size := range [][2]int{{80, 24}, {101, 37}} {
		if err := terminal.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		width, height, err := term.GetSize(int(terminal.(*unixPty).slave.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		if width != size[0] || height != size[1] {
			t.Fatalf("terminal size = %dx%d, want %dx%d", width, height, size[0], size[1])
		}
	}
}
