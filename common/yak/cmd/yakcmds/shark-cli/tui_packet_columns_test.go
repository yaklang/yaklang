package sharkcli

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPacketColumnsSeparateProtocolsAndUseAvailableEndpointSpace(t *testing.T) {
	for _, width := range []int{70, 80, 100, 160} {
		for _, protocol := range []string{"TLSv1.3", "VXLAN-GPE", "HTTP/2"} {
			u := &tui{}
			u.visible = []*packetEntry{{packet: &capturedPacket{}, summary: packetSummary{Number: 1, Time: time.Unix(1, 0), Protocol: protocol, Source: "127.0.0.1", Destination: "192.168.100.254", Length: 1388, Info: "actual protocol information"}}}
			c := newCanvas(width, 3)
			u.drawPackets(c, rect{0, 0, width, 3})
			line := strings.Split(c.render(false), "\r\n")[1]
			at := strings.Index(line, protocol)
			require.GreaterOrEqual(t, at, 0)
			require.Equal(t, byte(' '), line[at+len(protocol)], "a full protocol label must be separated from Source")
			require.Contains(t, line, "127.0.0.1", "the short source fits even in the compact list")
			require.Contains(t, strings.Split(c.render(false), "\r\n")[0], "Info")
			if width >= 80 {
				require.Contains(t, line, "192.168.100.254", "use spare Info width before cutting an ordinary IPv4 address")
			}
		}
	}
}

func TestPacketColumnsFitVisibleIPv6AndKeepAnInfoMinimumAfterResize(t *testing.T) {
	for _, width := range []int{62, 72, 92, 152} {
		widths := packetTableColumnWidths(width, 8, 39, 39)
		used := 6 // One explicit separator after every fixed column.
		for _, n := range widths {
			require.GreaterOrEqual(t, n, 0)
			used += n
		}
		minimum := 10
		if width >= 100 {
			minimum = 20
		}
		require.GreaterOrEqual(t, width-used, minimum)
		if width >= 152 {
			require.Equal(t, 39, widths[3])
			require.Equal(t, 39, widths[4])
		}
	}
}
