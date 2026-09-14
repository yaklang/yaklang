package lowhttp

import (
	"io"
	"testing"
)

func FuzzH2ResponseHeaderBlock(f *testing.F) {
	f.Add([]byte{0x88}, uint16(0), false)
	f.Add([]byte{0x80}, uint16(1), true)
	f.Add([]byte{0x3f, 0xe1, 0x1f}, uint16(2), false)
	f.Add([]byte{}, uint16(0), true)
	f.Fuzz(func(t *testing.T, block []byte, cut uint16, canceled bool) {
		if len(block) > 16384 {
			t.Skip()
		}
		c := newH2ReadLoopTestConn(t, io.Discard)
		cs := newH2ReadLoopTestStream(t, c, 1)
		if canceled {
			cs.abort()
		}
		rl := &http2ClientConnReadLoop{h2Conn: c}
		split := int(cut) % (len(block) + 1)
		rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, block[:split], false, true))
		if !c.isClosed() {
			rl.processHeaderFragment(block[split:], true)
		}
		if len(rl.headerFields) != 0 {
			t.Fatal("finished or failed header block retained fields")
		}
		if !canceled && !c.isClosed() && cs.streamErr == nil && cs.resp.StatusCode < 200 {
			t.Fatal("malformed response reported success")
		}
	})
}
