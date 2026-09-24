package sharkcli

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// SGR coordinates support wide terminals. Button-motion reporting captures
// scrollbar drags without flooding stdin with every hover movement.
const terminalEnter = "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[?1004h\x1b[?2004h"
const terminalLeave = "\x1b[?2004l\x1b[?1004l\x1b[?1002l\x1b[?1000l\x1b[?1006l\x1b[0m\x1b[?25h\x1b[?1049l"

type mouseAction uint8

const (
	mousePress mouseAction = iota
	mouseRelease
	mouseMotion
	mouseWheel
)

type mouseEvent struct {
	action              mouseAction
	x, y, button, delta int
	horizontal, shift   bool
}
type terminalEvent struct {
	key, paste string
	mouse      *mouseEvent
	focus      int
}

type inputDecoder struct {
	pending    []byte
	escapeAt   time.Time
	pasting    bool
	paste      []byte
	discardCSI bool
}

// Decode complete events, never individual characters of a terminal report.
// Readers may split an escape sequence or UTF-8 character at any byte boundary.
func (d *inputDecoder) feed(data []byte, now time.Time) []terminalEvent {
	d.pending = append(d.pending, data...)
	var events []terminalEvent
	for len(d.pending) > 0 {
		if d.pasting {
			end := bytes.Index(d.pending, []byte("\x1b[201~"))
			if end < 0 {
				// Keep a possible fragmented end marker for the next read.
				n := max(0, len(d.pending)-5)
				d.appendPaste(d.pending[:n])
				d.pending = d.pending[n:]
				break
			}
			d.appendPaste(d.pending[:end])
			events = append(events, terminalEvent{paste: string(d.paste)})
			d.pending = d.pending[end+6:]
			d.paste = nil
			d.pasting = false
			continue
		}
		if d.discardCSI {
			end := -1
			for i, b := range d.pending {
				if b >= 0x40 && b <= 0x7e {
					end = i
					break
				}
			}
			if end < 0 {
				d.pending = nil
				break
			}
			d.pending = d.pending[end+1:]
			d.discardCSI = false
			continue
		}
		if d.pending[0] != 27 {
			if !utf8.FullRune(d.pending) {
				break
			}
			r, n := utf8.DecodeRune(d.pending)
			events = append(events, terminalEvent{key: string(r)})
			d.pending = d.pending[n:]
			continue
		}
		if d.escapeAt.IsZero() {
			d.escapeAt = now
		}
		if len(d.pending) < 2 {
			break
		}
		if d.pending[1] != '[' && d.pending[1] != 'O' {
			// A standalone Escape immediately followed by a normal key preserves both.
			events = append(events, terminalEvent{key: "\x1b"})
			d.pending = d.pending[1:]
			d.escapeAt = time.Time{}
			continue
		}
		if len(d.pending) < 3 {
			break
		}
		if d.pending[1] == 'O' {
			end := -1
			for i := 2; i < len(d.pending); i++ {
				if d.pending[i] >= 0x40 && d.pending[i] <= 0x7e {
					end = i
					break
				}
			}
			if end < 0 {
				if len(d.pending) > 128 {
					d.pending = nil
					d.discardCSI = true
					d.escapeAt = time.Time{}
				}
				break
			}
			if key := cursorKey(d.pending[end]); key != "" {
				events = append(events, terminalEvent{key: key})
			}
			d.pending = d.pending[end+1:]
			d.escapeAt = time.Time{}
			continue
		}

		if d.pending[2] == 'M' { // Legacy X10 mouse report; coordinates are raw bytes.
			if len(d.pending) < 6 {
				break
			}
			if d.pending[3] >= 32 && d.pending[4] >= 33 && d.pending[5] >= 33 {
				if m := decodeMouse(int(d.pending[3])-32, int(d.pending[4])-33, int(d.pending[5])-33, false); m != nil {
					events = append(events, terminalEvent{mouse: m})
				}
			}
			d.pending = d.pending[6:]
			d.escapeAt = time.Time{}
			continue
		}
		end := -1
		for i := 2; i < len(d.pending); i++ {
			if d.pending[i] >= 0x40 && d.pending[i] <= 0x7e {
				end = i
				break
			}
		}
		if end < 0 {
			if len(d.pending) > 128 {
				d.pending = nil
				d.discardCSI = true
				d.escapeAt = time.Time{}
			}
			break
		}
		body := string(d.pending[2:end])
		final := d.pending[end]
		d.pending = d.pending[end+1:]
		d.escapeAt = time.Time{}
		switch {
		case strings.HasPrefix(body, "<") && (final == 'M' || final == 'm'):
			if m := parseMouseReport(body[1:], final == 'm', 0); m != nil {
				events = append(events, terminalEvent{mouse: m})
			}
		case final == 'M': // urxvt 1015 fallback.
			if m := parseMouseReport(body, false, 32); m != nil {
				events = append(events, terminalEvent{mouse: m})
			}
		case final == 'I' && body == "":
			events = append(events, terminalEvent{focus: 1})
		case final == 'O' && body == "":
			events = append(events, terminalEvent{focus: -1})
		case final == 'Z' && body == "":
			events = append(events, terminalEvent{key: "\x1b[Z"})
		case final == '~':
			code := strings.Split(body, ";")[0]
			if code == "200" {
				d.pasting = true
				d.paste = nil
				continue
			}
			if code == "1" || code == "7" {
				events = append(events, terminalEvent{key: "\x1b[H"})
			} else if code == "4" || code == "8" {
				events = append(events, terminalEvent{key: "\x1b[F"})
			} else if code == "5" || code == "6" {
				events = append(events, terminalEvent{key: "\x1b[" + code + "~"})
			}
		default:
			// CSI cursor modifiers (1;2A, 1;5B, …) keep their navigation semantics.
			if !strings.HasPrefix(body, "<") {
				if key := cursorKey(final); key != "" {
					events = append(events, terminalEvent{key: key})
				}
			}
		}
	}
	return events
}
func (d *inputDecoder) appendPaste(p []byte) {
	if available := (64 << 10) - len(d.paste); available > 0 {
		d.paste = append(d.paste, p[:min(available, len(p))]...)
	}
}
func (d *inputDecoder) expire(now time.Time) []terminalEvent {
	if !d.pasting && len(d.pending) == 1 && d.pending[0] == 27 && !d.escapeAt.IsZero() && now.Sub(d.escapeAt) >= 75*time.Millisecond {
		d.pending = nil
		d.escapeAt = time.Time{}
		return []terminalEvent{{key: "\x1b"}}
	}
	return nil
}
func cursorKey(final byte) string {
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F':
		return "\x1b[" + string(final)
	}
	return ""
}
func parseMouseReport(body string, released bool, offset int) *mouseEvent {
	parts := strings.Split(body, ";")
	if len(parts) != 3 {
		return nil
	}
	b, e1 := strconv.Atoi(parts[0])
	x, e2 := strconv.Atoi(parts[1])
	y, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || b < 0 || b > 255 || x < 1 || y < 1 || x > 100000 || y > 100000 {
		return nil
	}
	return decodeMouse(b-offset, x-1, y-1, released)
}
func decodeMouse(button, x, y int, released bool) *mouseEvent {
	if button < 0 || button >= 128 {
		return nil
	}
	m := &mouseEvent{x: x, y: y, button: button & 3, shift: button&4 != 0}
	switch {
	case released || button&3 == 3 && button&64 == 0:
		m.action = mouseRelease
	case button&64 != 0:
		m.action = mouseWheel
		m.delta = -3
		if button&1 != 0 {
			m.delta = 3
		}
		m.horizontal = button&2 != 0
	case button&32 != 0:
		m.action = mouseMotion
	default:
		m.action = mousePress
	}
	return m
}
func readTerminalInput(ctx context.Context, input io.Reader, output chan<- []byte) {
	defer close(output)
	buffer := make([]byte, 4096)
	for {
		n, err := input.Read(buffer)
		if n > 0 {
			data := append([]byte(nil), buffer[:n]...)
			select {
			case output <- data:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}
