/*
Ref: https://github.com/nhooyr/websocket/

Copyright (c) 2023 Anmol Sethi <hi@nhooyr.io>

Permission to use, copy, modify, and distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
*/
package lowhttp

import "sync"

type slidingWindow struct {
	buf []byte
}

var (
	swPoolMu sync.RWMutex
	swPool   = map[int]*sync.Pool{}
)

func slidingWindowPool(n int) *sync.Pool {
	swPoolMu.RLock()
	p, ok := swPool[n]
	swPoolMu.RUnlock()
	if ok {
		return p
	}

	p = &sync.Pool{}

	swPoolMu.Lock()
	swPool[n] = p
	swPoolMu.Unlock()

	return p
}

func (sw *slidingWindow) init(n int) {
	if sw.buf != nil {
		return
	}

	if n == 0 {
		n = 32768
	}

	p := slidingWindowPool(n)
	sw2, ok := p.Get().(*slidingWindow)
	if ok {
		*sw = *sw2
	} else {
		sw.buf = make([]byte, 0, n)
	}
}

func (sw *slidingWindow) close() {
	sw.buf = sw.buf[:0]
	swPoolMu.Lock()
	swPool[cap(sw.buf)].Put(sw)
	swPoolMu.Unlock()
}

func (sw *slidingWindow) write(p []byte) {
	if len(p) >= cap(sw.buf) {
		sw.buf = sw.buf[:cap(sw.buf)]
		p = p[len(p)-cap(sw.buf):]
		copy(sw.buf, p)
		return
	}

	left := cap(sw.buf) - len(sw.buf)
	if left < len(p) {
		// We need to shift spaceNeeded bytes from the end to make room for p at the end.
		spaceNeeded := len(p) - left
		copy(sw.buf, sw.buf[spaceNeeded:])
		sw.buf = sw.buf[:len(sw.buf)-spaceNeeded]
	}

	sw.buf = append(sw.buf, p...)
}
