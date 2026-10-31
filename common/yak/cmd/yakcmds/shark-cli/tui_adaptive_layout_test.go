package sharkcli

import (
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestSparseFilteredLayoutSharesRenderedAndInteractiveCoordinates(t *testing.T) {
	u := browsingFixture(t)
	ordinary := u.dashboardLayout()
	wire := udpPacket(t, 9999, []byte("unique-layout-match"))
	u.ring.add(&capturedPacket{number: 101, data: wire, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(wire)}})
	require.NoError(t, u.applySearch("content:unique-layout-match"))
	u.refreshVisible()
	u.syncInspection(time.Now())
	require.Len(t, u.visible, 1)
	longInspector(u)
	compact := u.dashboardLayout()
	require.GreaterOrEqual(t, compact.fields.h-ordinary.fields.h, 8, "spare packet rows expose materially more protocol detail")
	view := strings.Split(u.view(&captureSession{}, &captureSource{}, captureConfig{}), "\n")
	require.Contains(t, view[compact.tabs], "[2 Fields]")
	require.Contains(t, view[compact.tabs], "[4 Stream]")
	// This tab moved upward. Clicking its rendered location must still
	// target the field pane, and the wheel must target its larger viewport.
	u.mouse(mouseEvent{action: mousePress, button: 0, x: 5, y: compact.tabs})
	require.Equal(t, 1, u.pane)
	r := u.viewport(1)
	u.mouse(mouseEvent{action: mouseWheel, x: r.x + 4, y: r.y + 2, delta: 7})
	require.Equal(t, 7, u.fieldTop)
	require.Zero(t, u.hexTop)
	require.Equal(t, uint64(101), u.selected)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: r.x + 3, y: r.y + 2})
	require.Equal(t, 9, u.fieldSelection, "field hit testing uses the actual expanded viewport")
	u.mouse(mouseEvent{action: mousePress, button: 0, x: 38, y: compact.tabs})
	require.Equal(t, 3, u.pane)
	for _, size := range [][2]int{{80, 30}, {180, 48}, {70, 24}} {
		u.resize(size[0], size[1])
		layout := u.dashboardLayout()
		require.Less(t, layout.packets.y+layout.packets.h, layout.stream.y)
		require.Equal(t, layout.footer, layout.stream.y+layout.stream.h)
		require.Positive(t, u.viewport(3).h)
		u.mouse(mouseEvent{action: mousePress, button: 0, x: 5, y: layout.tabs})
		require.Equal(t, 1, u.pane)
	}
	// Clearing an active live filter restores the normal list geometry.
	require.NoError(t, u.applySearch(""))
	u.refreshVisible()
	require.Equal(t, layoutDashboard(u.width-1, u.height), u.dashboardLayout())
}

func TestSparseEOFAndEmptyResultsKeepHeadersAndMessageInsideTheirPanel(t *testing.T) {
	u := &tui{ring: newPacketRing(10), width: 180, height: 48, follow: true}
	base := u.dashboardLayout()
	u.visible = make([]*packetEntry, 1)
	require.Equal(t, base, u.dashboardLayout(), "unfiltered live arrivals do not shift the inspector")
	u.visible = nil
	u.finished = true
	l := u.dashboardLayout()
	view := strings.Split(u.view(&captureSession{}, &captureSource{offline: true}, captureConfig{}), "\n")
	message := -1
	for y, row := range view {
		if strings.Contains(row, "No matching packets") {
			message = y
		}
	}
	require.Greater(t, message, l.packets.y)
	require.Less(t, message, l.packets.y+l.packets.h-1, "empty-result guidance cannot overwrite the moved tabs or border")
	require.Contains(t, view[l.tabs], "[2 Fields]")
	require.Greater(t, l.fields.h, base.fields.h)
	u.visible = make([]*packetEntry, 100)
	require.Equal(t, base, u.dashboardLayout(), "busy replays retain their normal packet capacity")
}
