package sharkcli

import (
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func browsingFixture(t *testing.T) *tui {
	t.Helper()
	u := &tui{ring: newPacketRing(100), follow: true, width: 160, height: 48}
	p := dnsPacket(t)
	for i := 1; i <= 100; i++ {
		u.ring.add(&capturedPacket{number: uint64(i), data: p, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(p)}})
	}
	u.refreshVisible()
	u.syncInspection(time.Now())
	u.view(&captureSession{}, &captureSource{}, captureConfig{})
	return u
}
func longInspector(u *tui) {
	u.fields.fields = nil
	for i := 0; i < 200; i++ {
		u.fields.fields = append(u.fields.fields, field{text: fmt.Sprintf("field %03d", i)})
	}
	// Use a private packet instead of mutating capture data referenced by the ring.
	raw := *u.inspect
	raw.data = make([]byte, 8192)
	raw.ci.Length = len(raw.data)
	u.inspect = &raw
}
func TestWheelScrollIsIndependentAndListRemainsFrozenUnderEviction(t *testing.T) {
	u := browsingFixture(t)
	r := u.viewport(0)
	initial := u.listTop
	selected := u.selected
	u.mouse(mouseEvent{action: mouseWheel, x: r.x + 5, y: r.y + 4, delta: -3})
	require.False(t, u.follow)
	require.NotNil(t, u.frozen)
	require.Equal(t, initial-3, u.listTop)
	require.Equal(t, selected, u.selected)
	topNumber := u.visible[u.listTop].packet.number
	raw := u.inspect
	for i := 101; i <= 400; i++ {
		p := *raw
		p.number = uint64(i)
		u.ring.add(&p)
	}
	u.refreshVisible()
	u.syncInspection(time.Now())
	for i := 0; i < 5; i++ {
		u.view(&captureSession{}, &captureSource{}, captureConfig{})
	}
	require.Equal(t, initial-3, u.listTop)
	require.Equal(t, topNumber, u.visible[u.listTop].packet.number)
	require.Equal(t, selected, u.fields.number)
	u.key(" ", &captureSource{})
	u.refreshVisible()
	u.syncInspection(time.Now())
	u.view(&captureSession{}, &captureSource{}, captureConfig{})
	require.True(t, u.follow)
	require.Nil(t, u.frozen)
	require.Equal(t, uint64(400), u.selected)
}
func TestWheelTargetsHoveredPaneAndDoesNotFollowSelection(t *testing.T) {
	u := browsingFixture(t)
	longInspector(u)
	listTop := u.listTop
	selected := u.selected
	fields := u.viewport(1)
	hex := u.viewport(2)
	u.mouse(mouseEvent{action: mouseWheel, x: fields.x + 4, y: fields.y + 2, delta: 6})
	require.Equal(t, 1, u.pane)
	require.Equal(t, 6, u.fieldTop)
	require.Equal(t, 0, u.fieldSelection)
	require.Equal(t, listTop, u.listTop)
	require.Equal(t, 0, u.hexTop)
	u.mouse(mouseEvent{action: mouseWheel, x: hex.x + 4, y: hex.y + 2, delta: 9})
	require.Equal(t, 2, u.pane)
	require.Equal(t, 9, u.hexTop)
	require.Equal(t, 6, u.fieldTop)
	require.Equal(t, selected, u.selected)
	for i := 0; i < 4; i++ {
		u.view(&captureSession{}, &captureSource{}, captureConfig{})
	}
	require.Equal(t, 6, u.fieldTop)
	require.Equal(t, 9, u.hexTop)
	// Horizontal trackpad and outside-panel scrolling must not move another pane.
	u.mouse(mouseEvent{action: mouseWheel, x: hex.x, y: hex.y, delta: 3, horizontal: true})
	u.mouse(mouseEvent{action: mouseWheel, x: 5, y: 0, delta: 3})
	require.Equal(t, 9, u.hexTop)
}
func TestClickUsesVisibleRowAndFlatFieldSelection(t *testing.T) {
	u := browsingFixture(t)
	u.scroll(0, -15)
	r := u.viewport(0)
	number := u.visible[u.listTop+3].packet.number
	u.mouse(mouseEvent{action: mousePress, x: r.x + 15, y: r.y + 3, button: 0})
	u.syncInspection(time.Now())
	require.Equal(t, number, u.selected)
	require.Equal(t, number, u.fields.number)
	u.fields.fields = []field{{text: "root", branch: true}, {depth: 1, text: "child"}, {text: "next"}}
	r = u.viewport(1)
	u.mouse(mouseEvent{action: mousePress, x: r.x + 1, y: r.y, button: 0})
	require.Len(t, u.fieldIndices(), 3)
	u.key("j", &captureSource{})
	require.Equal(t, 1, u.fieldSelection)
	u.key("k", &captureSource{})
	require.Equal(t, 0, u.fieldSelection)
}
func TestScrollbarDragRemainsBoundToItsPaneAndReleases(t *testing.T) {
	u := browsingFixture(t)
	longInspector(u)
	r := u.viewport(1)
	other := u.listTop
	u.mouse(mouseEvent{action: mousePress, x: r.x + r.w - 1, y: r.y, button: 0})
	require.True(t, u.drag.active)
	// Dragging outside the panel still controls the original scrollbar.
	u.mouse(mouseEvent{action: mouseMotion, x: 0, y: r.y + r.h + 20, button: 0})
	require.Equal(t, u.rowCount(1)-r.h, u.fieldTop)
	require.Equal(t, other, u.listTop)
	u.mouse(mouseEvent{action: mouseRelease, x: 0, y: 0})
	require.False(t, u.drag.active)
	top := u.fieldTop
	u.mouse(mouseEvent{action: mouseMotion, x: 0, y: r.y, button: 0})
	require.Equal(t, top, u.fieldTop)
	u.drag = scrollbarDrag{active: true, pane: 1}
	u.event(terminalEvent{focus: -1}, &captureSource{})
	require.False(t, u.drag.active)
}
func TestResizeKeepsByteOffsetAndEndReachesLastByteAtEveryWidth(t *testing.T) {
	u := browsingFixture(t)
	longInspector(u)
	u.focusPane(2)
	u.setTop(2, 7)
	offset := u.hexTop * u.hexPerRow()
	for _, width := range []int{100, 160, 260} {
		u.resize(width, 48)
		require.LessOrEqual(t, u.hexTop*u.hexPerRow(), offset)
		require.Less(t, offset-u.hexTop*u.hexPerRow(), u.hexPerRow())
		offset = u.hexTop * u.hexPerRow()
	}
	for _, width := range []int{80, 120, 160, 320} {
		u.resize(width, 48)
		u.key("G", &captureSource{})
		rows := u.viewport(2).h
		require.GreaterOrEqual(t, (u.hexTop+rows)*u.hexPerRow(), len(u.inspect.data))
		u.view(&captureSession{}, &captureSource{}, captureConfig{})
		require.Equal(t, max(0, u.rowCount(2)-rows), u.hexTop)
	}
}
func TestDeepDecodeDoesNotResetAnInteractedTree(t *testing.T) {
	u := browsingFixture(t)
	longInspector(u)
	u.scroll(1, 12)
	old := u.fields.fields[0].text
	d := packetDetail{number: u.selected, fields: []field{{text: "full tree", branch: true}, {depth: 1, text: "full child"}}}
	u.applyDetail(detailResult{number: u.selected, detail: d})
	require.Equal(t, 12, u.fieldTop)
	require.Equal(t, old, u.fields.fields[0].text)
	require.NotNil(t, u.pendingDetail)
	u.key("d", &captureSource{})
	require.Equal(t, "full tree", u.fields.fields[0].text)
	require.Zero(t, u.fieldTop)
	require.Nil(t, u.pendingDetail)
}
