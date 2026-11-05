package sharkcli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fieldTextRow(u *tui) string {
	c := newCanvas(u.width, u.height)
	r := u.viewport(1)
	u.drawFields(c, r)
	var text strings.Builder
	for x := r.x; x < r.x+r.w-2; x++ {
		text.WriteString(c.cells[r.y*c.width+x].text)
	}
	return text.String()
}

func TestFieldTextScrollingRevealsActualGREASEErrorSuffix(t *testing.T) {
	u := browsingFixture(t)
	u.resize(180, 48)
	u.focusPane(1)
	u.fields.fields = []field{{depth: 1, text: "Error: TLS CertificateVerify selects reserved GREASE signature algorithm 0x7a7a"}}
	u.fieldSelection, u.fieldTop = 0, 0
	before := fieldTextRow(u)
	require.Contains(t, before, "Error: TLS CertificateVerify")
	require.NotContains(t, before, "0x7a7a", "this is the observed Windows clipping regression")
	for i := 0; i < 64; i++ {
		require.False(t, u.key("\x1b[C", &captureSource{}))
	}
	require.Positive(t, u.fieldLeft)
	require.Equal(t, u.fieldTextLimit(), u.fieldLeft)
	after := fieldTextRow(u)
	require.True(t, strings.HasPrefix(after, "‹"))
	require.Contains(t, after, "0x7a7a")
	require.NotContains(t, after, "…", "the last value must be completely reachable")
	for i := 0; i < 64; i++ {
		u.key("\x1b[D", &captureSource{})
	}
	require.Zero(t, u.fieldLeft)
	require.Equal(t, before, fieldTextRow(u))
	require.Contains(t, u.view(&captureSession{}, &captureSource{}, captureConfig{}), "←→ field text")
}

func TestFieldTextColumnWindowHandlesWideAndSanitizedCharacters(t *testing.T) {
	for _, tc := range []struct {
		text    string
		columns int
		want    string
	}{
		{"A中🙂Z", 0, "A中🙂Z"}, {"A中🙂Z", 1, "‹中🙂Z"},
		{"A中🙂Z", 2, "‹ 🙂Z"}, {"A中🙂Z", 3, "‹🙂Z"},
		{"A中🙂Z", 4, "‹ Z"}, {"A中🙂Z", 5, "‹Z"},
		{"A中🙂Z", 6, "‹"}, {"", 1, "‹"},
	} {
		t.Run(fmt.Sprint(tc.columns)+tc.text, func(t *testing.T) {
			require.Equal(t, tc.want, fieldTextAfterColumns(tc.text, tc.columns))
		})
	}
	text := fieldDisplayText(field{depth: 1, text: "Error: \x1b[31mline\n尾部"})
	require.NotContains(t, text, "\x1b")
	require.NotContains(t, text, "\n")
}

func TestFieldTextScrollDoesNotAlterSelectionSourceOrHighlight(t *testing.T) {
	u := browsingFixture(t)
	u.focusPane(1)
	owned := []byte{0, 1, 2, 3, 0x7a, 0x7a, 6, 7}
	u.fields.sources = []detailSource{{Name: "Owned record", Data: bytes.Clone(owned)}}
	u.fields.fields = []field{{text: strings.Repeat("long protocol field ", 12), source: 1, start: 4, end: 6, hasRange: true}, {text: "short field"}}
	u.fieldSelection, u.fieldTop = 0, 0
	require.True(t, u.selectFieldBytes())
	packet, selected, source, top, first, last := u.inspect, u.selected, u.bytesSource, u.hexTop, u.hexStart, u.hexEnd
	u.key("\x1b[C", &captureSource{})
	require.Positive(t, u.fieldLeft)
	require.Same(t, packet, u.inspect)
	require.Equal(t, selected, u.selected)
	require.Equal(t, source, u.bytesSource)
	require.Equal(t, top, u.hexTop)
	require.Equal(t, first, u.hexStart)
	require.Equal(t, last, u.hexEnd)
	require.Equal(t, owned, u.byteData())
	u.move(1)
	require.Equal(t, 1, u.fieldSelection)
	require.Zero(t, u.fieldLeft)
	u.move(-1)
	u.scrollFieldText(10000)
	u.resize(1000, 48)
	require.Zero(t, u.fieldLeft, "a wider viewport must restore the complete prefix")
	u.resize(80, 30)
	u.scrollFieldText(10000)
	require.Contains(t, fieldTextRow(u), "field ")
	u.installDetail(packetDetail{fields: []field{{text: "new packet"}}})
	require.Zero(t, u.fieldLeft)
}

func TestFieldTextHorizontalWheelTargetsOnlyFields(t *testing.T) {
	for _, shifted := range []bool{false, true} {
		t.Run(fmt.Sprint(shifted), func(t *testing.T) {
			u := browsingFixture(t)
			u.fields.fields = []field{{text: strings.Repeat("value ", 40)}}
			u.fieldSelection, u.fieldTop = 0, 0
			r := u.viewport(1)
			u.mouse(mouseEvent{action: mouseWheel, x: r.x, y: r.y, delta: 2, horizontal: !shifted, shift: shifted})
			require.Equal(t, 1, u.pane)
			require.Equal(t, 8, u.fieldLeft)
			require.Zero(t, u.fieldTop)
			left := u.fieldLeft
			u.pane = 2
			u.key("\x1b[C", &captureSource{})
			r = u.viewport(2)
			u.mouse(mouseEvent{action: mouseWheel, x: r.x, y: r.y, delta: 3, horizontal: true})
			require.Equal(t, left, u.fieldLeft)
			require.Zero(t, u.hexTop)
		})
	}
}
