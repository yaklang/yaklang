package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

const natsTestRule = "application-layer.nats"

func TestProtocolCorpusNATSRuleLayouts(t *testing.T) {
	info := []byte("INFO {\"server_id\":\"local\",\"headers\":true}\r\n")
	line := protocolCorpusRequireBoundedRuleParse(t, info, natsTestRule, "NATS")
	protocolCorpusRequireValue(t, line, "Operation", "INFO")
	protocolCorpusRequireValue(t, line, "Arguments", "{\"server_id\":\"local\",\"headers\":true}")
	for _, generic := range [][]byte{[]byte("HELLO world\r\n"), []byte("PUB foo 3\r\n")} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(generic), natsTestRule, "NATS")
		require.Error(t, err, "%q is not a standalone NATS signature", generic)
	}

	control := []byte("PING\r\n")
	controlNode := protocolCorpusRequireBoundedRuleParse(t, control, natsTestRule, "NATSControlLine")
	protocolCorpusRequireValue(t, controlNode, "Control Line", "PING")

	body := []byte{'A', '\r', '\n', 0, 'B'}
	wire := append([]byte("PUB foo.bar 5\r\n"), body...)
	wire = append(wire, '\r', '\n')
	message := protocolCorpusRequireBoundedRuleParse(t, wire, natsTestRule, "NATSPayloadFrame")
	protocolCorpusRequireValue(t, message, "Control Line", "PUB foo.bar 5")
	protocolCorpusRequireValue(t, message, "Payload and Trailer", []byte{'A', '\r', '\n', 0, 'B', '\r', '\n'})

	for _, entryWire := range [][]byte{control[:len(control)-1], []byte("PING\n")} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(entryWire), natsTestRule, "NATSControlLine")
		require.Error(t, err)
	}
}
