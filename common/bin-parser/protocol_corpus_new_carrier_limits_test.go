package bin_parser

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

type sampledCarrierLimitReader struct {
	bits  uint64
	reads int
}

func (r *sampledCarrierLimitReader) InputBitLength() uint64 { return r.bits }
func (r *sampledCarrierLimitReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

// A failed structured candidate must not bypass the same resource ceiling by
// immediately allocating an arbitrarily large raw fallback. These are only
// this implementation batch's carrier boundaries, not a whole-library suite.
func TestProtocolCorpusNewCarrierResourceLimits(t *testing.T) {
	for _, test := range []struct {
		rule, entry, raw string
		limit            int
	}{
		{"application-layer.gquic", "GQUIC35ClientHelloCarrier", "Unparsed GQUIC35 Client Hello", 1452},
		{"application-layer.smtp_reply", "SMTPReplyCarrier", "Unparsed SMTP Reply", 524288},
		{"application-layer.browser_mailslot", "BrowserMailslotCarrier", "Unparsed Browser Mailslot Datagram", 65527},
		{"application-layer.browser_mailslot", "BrowserMailslotStrictCarrier", "Unparsed Browser Mailslot Datagram", 65527},
	} {
		t.Run(test.entry, func(t *testing.T) {
			reader := &sampledCarrierLimitReader{bits: uint64(test.limit+1) * 8}
			_, err := parser.ParseBinary(reader, test.rule, test.entry)
			require.ErrorContains(t, err, "resource limit")
			require.Zero(t, reader.reads, "oversized carrier read before its resource guard")
			// The exact maximum remains a valid raw fallback boundary, even
			// when none of the structured profile's fields can be decoded.
			wire := bytes.Repeat([]byte{0xff}, test.limit)
			node := protocolCorpusRequireBoundedRuleParse(t, wire, test.rule, test.entry)
			require.Equal(t, wire, NodeToBytes(node))
			protocolCorpusRequireValue(t, node, test.raw, wire)
		})
	}
}
