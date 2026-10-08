package sharkcli

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

type ink uint8

const (
	inkText ink = iota
	inkMuted
	inkBorder
	inkCyan
	inkBlue
	inkGreen
	inkGold
	inkRed
	inkTitle
	inkSelected
	inkHeader
)

var inks = []string{
	"38;2;195;210;225;48;2;11;17;27", "38;2;126;151;169;48;2;11;17;27", "38;2;54;86;108;48;2;11;17;27",
	"38;2;93;218;224;48;2;11;17;27", "38;2;124;168;255;48;2;11;17;27", "38;2;121;219;167;48;2;11;17;27",
	"38;2;240;202;120;48;2;11;17;27", "38;2;248;137;145;48;2;11;17;27", "1;38;2;227;237;247;48;2;11;17;27",
	"1;38;2;222;248;255;48;2;30;74;94", "38;2;136;178;195;48;2;19;31;44",
}

type cell struct {
	text  string
	style ink
}
type canvas struct {
	width, height int
	cells         []cell
}
type rect struct{ x, y, w, h int }

func newCanvas(w, h int) *canvas {
	c := &canvas{width: max(1, w), height: max(1, h)}
	c.cells = make([]cell, c.width*c.height)
	for i := range c.cells {
		c.cells[i].text = " "
	}
	return c
}
func (c *canvas) put(x, y int, text string, style ink) {
	if y < 0 || y >= c.height {
		return
	}
	for _, r := range safeText(text) {
		width := runewidth.RuneWidth(r)
		if width == 0 {
			continue
		}
		if x+width > c.width {
			return
		}
		if x >= 0 {
			c.cells[y*c.width+x] = cell{string(r), style}
			for j := 1; j < width; j++ {
				c.cells[y*c.width+x+j] = cell{"", style}
			}
		}
		x += width
	}
}
func (c *canvas) text(x, y, w int, text string, style ink) {
	if w > 0 {
		c.put(x, y, runewidth.Truncate(text, w, "…"), style)
	}
}
func (c *canvas) fill(x, y, w int, style ink) { c.put(x, y, strings.Repeat(" ", max(0, w)), style) }
func (c *canvas) box(r rect, title, right string, active bool) rect {
	border := inkBorder
	if active {
		border = inkCyan
	}
	c.put(r.x, r.y, "╭"+strings.Repeat("─", max(0, r.w-2))+"╮", border)
	c.put(r.x, r.y+r.h-1, "╰"+strings.Repeat("─", max(0, r.w-2))+"╯", border)
	for y := r.y + 1; y < r.y+r.h-1; y++ {
		c.put(r.x, y, "│", border)
		c.put(r.x+r.w-1, y, "│", border)
	}
	c.text(r.x+2, r.y, r.w-4, " "+title+" ", inkTitle)
	if right != "" && runewidth.StringWidth(title)+runewidth.StringWidth(right)+9 < r.w {
		c.put(r.x+r.w-runewidth.StringWidth(right)-3, r.y, " "+right+" ", border)
	}
	return panelContent(r)
}
func (c *canvas) render(color bool) string {
	var b strings.Builder
	for y := 0; y < c.height; y++ {
		if y > 0 {
			b.WriteString("\r\n")
		}
		last := ink(255)
		for x := 0; x < c.width; x++ {
			cell := c.cells[y*c.width+x]
			if color && cell.style != last {
				b.WriteString("\x1b[0;")
				b.WriteString(inks[cell.style])
				b.WriteByte('m')
				last = cell.style
			}
			b.WriteString(cell.text)
		}
		if color {
			b.WriteString("\x1b[0m")
		}
	}
	return b.String()
}

// Absolute row addressing avoids wrap/scroll damage and only updates changed
// lines. No logger or decoder shares the terminal output stream.
type screenWriter struct {
	out           io.Writer
	previous      []string
	width, height int
}

func (s *screenWriter) draw(frame string, w, h int) error {
	rows := strings.Split(frame, "\r\n")
	var output strings.Builder
	if s.width != w || s.height != h {
		s.previous = nil
		output.WriteString("\x1b[2J")
		s.width = w
		s.height = h
	}
	for i, row := range rows {
		if i >= h {
			break
		}
		if i < len(s.previous) && s.previous[i] == row {
			continue
		}
		fmt.Fprintf(&output, "\x1b[%d;1H%s\x1b[K", i+1, row)
	}
	s.previous = rows
	if output.Len() == 0 {
		return nil
	}
	_, err := io.WriteString(s.out, output.String())
	return err
}

type trafficRate struct {
	start, last                            time.Time
	bytes, packets                         uint64
	bytesPerSecond, packetsPerSecond, peak float64
	history                                []float64
}

func (r *trafficRate) reset(now time.Time, s *captureSession) {
	r.start = now
	r.last = now
	r.bytes = s.bytes.Load()
	r.packets = s.captured.Load()
}
func (r *trafficRate) sample(now time.Time, s *captureSession) bool {
	if r.last.IsZero() {
		r.reset(now, s)
		return false
	}
	elapsed := now.Sub(r.last).Seconds()
	if elapsed < 1 {
		return false
	}
	bytes, packets := s.bytes.Load(), s.captured.Load()
	r.bytesPerSecond = float64(bytes-r.bytes) / elapsed
	r.packetsPerSecond = float64(packets-r.packets) / elapsed
	r.peak = math.Max(r.peak, r.bytesPerSecond)
	r.history = append(r.history, r.bytesPerSecond)
	if len(r.history) > 240 {
		r.history = append(r.history[:0], r.history[len(r.history)-240:]...)
	}
	r.bytes = bytes
	r.packets = packets
	r.last = now
	return true
}

type protocolShare struct {
	name  string
	count int
}

func (u *tui) updateMix() {
	counts := map[string]int{}
	u.mixTotal = min(128, u.ring.size)
	for i := u.ring.size - u.mixTotal; i < u.ring.size; i++ {
		counts[u.packetRow(u.ring.at(i)).Protocol]++
	}
	u.mix = nil
	for name, count := range counts {
		u.mix = append(u.mix, protocolShare{name, count})
	}
	sort.Slice(u.mix, func(i, j int) bool {
		if u.mix[i].count != u.mix[j].count {
			return u.mix[i].count > u.mix[j].count
		}
		return u.mix[i].name < u.mix[j].name
	})
}
func byteSize(v float64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
func compactNumber(v uint64) string {
	if v >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(v)/1e6)
	}
	if v >= 10000 {
		return fmt.Sprintf("%.1fk", float64(v)/1000)
	}
	return fmt.Sprint(v)
}
func protocolInk(name string) ink {
	switch strings.TrimPrefix(strings.ToUpper(name), "TCP/") {
	case "TCP", "TLS", "HTTPS":
		return inkBlue
	case "UDP", "QUIC":
		return inkCyan
	case "DNS", "MDNS", "LLMNR":
		return inkGreen
	case "HTTP", "HTTP2":
		return inkGold
	case "ICMPV4", "ICMPV6", "ARP":
		return inkRed
	}
	return inkText
}

type dashboardLayout struct {
	traffic, mix, health, packets, fields, bytes, stream rect
	tabs                                                 int
	footer                                               int
	split                                                bool
}

func layoutDashboard(w, h int) dashboardLayout {
	usable := w - 2
	top := clamp(h/5, 7, 11)
	l := dashboardLayout{footer: h - 3, split: w >= 118}
	if w >= 118 {
		a := usable * 48 / 100
		b := usable * 24 / 100
		l.traffic = rect{1, 2, a, top}
		l.mix = rect{a + 1, 2, b, top}
		l.health = rect{a + b + 1, 2, usable - a - b, top}
	} else {
		a := usable * 57 / 100
		l.traffic = rect{1, 2, a, top}
		l.health = rect{a + 1, 2, usable - a, top}
	}
	remaining := l.footer - (2 + top)
	packetHeight := max(6, remaining*53/100)
	detailHeight := remaining - packetHeight
	l.packets = rect{1, 2 + top, usable, packetHeight}
	y := l.packets.y + packetHeight
	l.tabs = y
	y++
	detailHeight--
	l.stream = rect{1, y, usable, detailHeight}
	if l.split {
		a := usable * 48 / 100
		l.fields = rect{1, y, a, detailHeight}
		l.bytes = rect{a + 1, y, usable - a, detailHeight}
	} else {
		l.fields = rect{1, y, usable, detailHeight}
		l.bytes = l.fields
	}
	return l
}

func (u *tui) view(s *captureSession, src *captureSource, cfg captureConfig) string {
	width, height := max(1, u.width-1), max(1, u.height)
	c := newCanvas(width, height)
	if width < 69 || height < 24 {
		c.text(1, 1, width-2, "YAK SHARK · resize to at least 70 × 24", inkCyan)
		c.text(1, 3, width-2, "Capture continues. q quits; Ctrl+C stops safely.", inkText)
		return c.render(u.color)
	}
	l := layoutDashboard(width, height)
	u.drawSearch(c)
	state := "● LIVE"
	if src.offline {
		state = "◈ FILE"
	}
	if u.finished {
		state = "■ STOPPED"
		if src.offline {
			state = "◈ EOF"
		}
		if u.finishErr != nil {
			state = "! ERROR"
		}
	}
	c.put(2, 0, "Y A K  S H A R K", inkCyan)
	c.put(21, 0, state, inkGreen)
	source := src.name
	if src.gateway != "" {
		source += "  →  " + src.gateway
	}
	c.text(33, 0, width-35, source, inkText)
	rateLabel := fmt.Sprintf("%.0f pkt/s", u.rate.packetsPerSecond)
	if src.offline {
		rateLabel = "file replay"
	}
	traffic := c.box(l.traffic, "traffic", rateLabel, false)
	u.drawTraffic(c, traffic, src.offline)
	health := c.box(l.health, "capture", "", false)
	healthRows := []struct {
		k, v  string
		style ink
	}{
		{"packets", compactNumber(s.captured.Load()), inkText}, {"captured", byteSize(float64(s.bytes.Load())), inkText},
		{"buffer", fmt.Sprintf("%d / %d", u.ring.size, len(u.ring.entries)), inkCyan},
		{"UI skipped", compactNumber(s.skipped.Load()), inkMuted}, {"kernel drops", compactNumber(s.kernelDropped.Load()), inkGreen},
		{"evicted", compactNumber(u.ring.evicted), inkMuted},
	}
	for i, row := range healthRows {
		if i >= health.h {
			break
		}
		if row.k == "kernel drops" && s.kernelDropped.Load() > 0 {
			row.style = inkRed
		}
		c.text(health.x, health.y+i, health.w/2, row.k, inkMuted)
		c.text(health.x+health.w/2, health.y+i, health.w-health.w/2, row.v, row.style)
	}
	if l.mix.w > 0 {
		r := c.box(l.mix, "protocols", fmt.Sprintf("sample %d", u.mixTotal), false)
		if len(u.mix) == 0 {
			c.text(r.x, r.y+1, r.w, "Waiting for traffic…", inkMuted)
		}
		for i, p := range u.mix {
			if i >= r.h {
				break
			}
			pct := float64(p.count) / float64(max(1, u.mixTotal))
			nameWidth := min(10, r.w/3)
			c.text(r.x, r.y+i, nameWidth, p.name, protocolInk(p.name))
			barWidth := max(1, r.w-nameWidth-8)
			x := r.x + nameWidth + 1
			c.put(x, r.y+i, strings.Repeat("━", barWidth), inkBorder)
			c.put(x, r.y+i, strings.Repeat("━", int(math.Round(pct*float64(barWidth)))), protocolInk(p.name))
			c.text(x+barWidth+1, r.y+i, 6, fmt.Sprintf("%3.0f%%", pct*100), inkText)
		}
	}
	mode := "following"
	if !u.follow {
		mode = "pinned"
		if u.frozen != nil {
			mode = "browsing · Space live"
		}
	}
	list := c.box(l.packets, "1 packets", fmt.Sprintf("%s · %d visible", mode, len(u.visible)), u.pane == 0)
	u.drawPackets(c, list)
	for i, label := range []string{"[2 Fields]", "[3 Bytes]", "[4 Stream]"} {
		style := inkMuted
		if u.pane == i+1 {
			style = inkSelected
		}
		c.put([]int{3, 20, 36}[i], l.tabs, label, style)
	}
	if u.pane == 3 {
		for i, label := range []string{"[h Beginning]", "[l Recent]", "[e Evidence]"} {
			x := []int{54, 71, 85}[i]
			if x+len(label) < width {
				style := inkMuted
				if i == 2 && u.streamMode == 3 || i == 0 && u.streamBeginning && u.streamMode != 3 || i == 1 && !u.streamBeginning && u.streamMode != 3 {
					style = inkSelected
				}
				c.put(x, l.tabs, label, style)
			}
		}
		mode := "auto refresh"
		if u.streamPinned {
			mode = "pinned"
		}
		direction := []string{"both directions", "A → B", "B → A"}[u.streamDirection]
		rangeLabel := "recent window"
		if u.streamBeginning {
			rangeLabel = "preserved beginning"
		}
		if u.streamMode == 3 {
			rangeLabel = "evidence"
		}
		r := c.box(l.stream, "4 STREAM", rangeLabel+" · "+mode+" · "+direction, true)
		u.drawStream(c, r)
	}
	if u.pane != 3 && (l.split || u.pane != 2) {
		r := c.box(l.fields, "2 PROTOCOL FIELDS", fmt.Sprintf("#%d", u.fields.number), u.pane == 1)
		u.drawFields(c, r)
	}
	if u.pane != 3 && (l.split || u.pane == 2) {
		r := c.box(l.bytes, "3 PACKET BYTES", u.bytesLabel(), u.pane == 2)
		u.drawBytes(c, r)
	}
	capture := cfg.filter
	if capture == "" {
		capture = "all"
	}
	display := u.filterText
	if display == "" {
		display = "all"
	}
	c.text(2, l.footer, width-4, "capture "+capture+"   │   display "+display, inkMuted)
	notice := u.decodeStatus
	if u.pane == 3 {
		notice = u.streamStatus
	}
	if cfg.output != "" {
		notice = "recording → " + cfg.output + "   │   " + notice
	}
	if u.notice != "" {
		notice = u.notice
	}
	if u.editor != "" {
		notice = u.editor + " > " + u.input + "▏   Enter apply · Esc cancel · Ctrl+U clear"
		if strings.HasPrefix(u.notice, "Search error:") || strings.HasPrefix(u.notice, "Cache error:") || strings.HasPrefix(u.notice, "Filter error:") {
			notice = u.notice + " · " + notice
		}
	}
	if u.finishErr != nil && u.editor == "" {
		notice = u.finishErr.Error()
	}
	c.text(2, l.footer+1, width-4, notice, inkGold)
	help := "q quit  / search  f BPF  c cache  wheel scroll pane  ↑↓ move  Tab pane  4 Stream  Space live  p protocol  d fields"
	if u.pane == 3 {
		help = "h beginning  l recent  e evidence  v A/B  t text  x hex  d fields  r refresh  / search  c cache  q quit"
	}
	c.text(2, l.footer+2, width-4, help, inkCyan)
	return c.render(u.color)
}

func (u *tui) drawTraffic(c *canvas, r rect, offline bool) {
	label := byteSize(u.rate.bytesPerSecond) + "/s"
	if offline {
		label = "OFFLINE CAPTURE"
	}
	c.text(r.x, r.y, r.w, label, inkCyan)
	if !offline {
		c.text(r.x+r.w/2, r.y, r.w-r.w/2, "peak "+byteSize(u.rate.peak)+"/s", inkMuted)
	}
	plot := rect{r.x, r.y + 1, r.w, max(1, r.h-2)}
	maximum := 1.0
	history := u.rate.history
	if offline {
		history = nil
	}
	if len(history) > plot.w {
		history = history[len(history)-plot.w:]
	}
	for _, v := range history {
		maximum = math.Max(maximum, v)
	}
	glyphs := []rune(" ▁▂▃▄▅▆▇█")
	for x := 0; x < plot.w; x++ {
		v := 0.0
		i := x - (plot.w - len(history))
		if i >= 0 {
			v = history[i]
		}
		scaled := v / maximum * float64(plot.h*8)
		for y := 0; y < plot.h; y++ {
			level := clamp(int(math.Round(scaled-float64((plot.h-1-y)*8))), 0, 8)
			ch := string(glyphs[level])
			style := inkCyan
			if level == 0 && y == plot.h-1 {
				ch = "┈"
				style = inkBorder
			}
			c.put(plot.x+x, plot.y+y, ch, style)
		}
	}
	caption := "1s intervals · captured bytes"
	if offline {
		caption = "File replay · rates disabled"
	}
	c.text(r.x, r.y+r.h-1, r.w, caption, inkMuted)
}

func (u *tui) drawPackets(c *canvas, r rect) {
	count := max(1, r.h-1)
	if u.follow {
		u.listTop = max(0, len(u.visible)-count)
	}
	u.listTop = clamp(u.listTop, 0, max(0, len(u.visible)-count))
	u.drawScrollbar(c, rect{r.x, r.y + 1, r.w, r.h - 1}, 0)
	r.w -= 2
	// Fit the endpoints first; the information column consumes the remaining width.
	compact := r.w < 100
	widths := []int{9, 12, 10, clamp((r.w-53)/3, 12, 39), clamp((r.w-53)/3, 12, 39), 6}
	if compact {
		widths = []int{7, 9, 8, max(10, (r.w-35)/2), max(10, (r.w-35)/2), 5}
	}
	labels := []string{"No.", "Time", "Protocol", "Source", "Destination", "Bytes"}
	c.fill(r.x, r.y, r.w, inkHeader)
	x := r.x
	for i, label := range labels {
		c.text(x, r.y, min(widths[i], r.x+r.w-x), label, inkHeader)
		x += widths[i] + 1
	}
	if x < r.x+r.w {
		c.text(x, r.y, r.x+r.w-x, "Info", inkHeader)
	}
	if len(u.visible) == 0 {
		c.text(r.x+2, r.y+2, r.w-4, "No matching packets · / search · click Clear · f BPF", inkMuted)
		return
	}
	for i := 0; i < count; i++ {
		at := u.listTop + i
		if at >= len(u.visible) {
			break
		}
		row := u.packetRow(u.visible[at])
		y := r.y + 1 + i
		selected := row.Number == u.selected
		style := inkText
		if selected {
			style = inkSelected
			c.fill(r.x, y, r.w, style)
		}
		timestamp := row.Time.Format("15:04:05.000")
		if compact {
			timestamp = row.Time.Format("15:04:05")
		}
		number := fmt.Sprint(row.Number)
		if selected {
			number = "›" + number
		}
		values := []string{number, timestamp, row.Protocol, row.Source, row.Destination, fmt.Sprint(row.Length)}
		x := r.x
		for j, value := range values {
			colStyle := style
			if !selected && j == 2 {
				colStyle = protocolInk(row.Protocol)
			}
			if !selected && (j == 0 || j == 1 || j == 5) {
				colStyle = inkMuted
			}
			c.text(x, y, min(widths[j], r.x+r.w-x), value, colStyle)
			x += widths[j] + 1
		}
		if x < r.x+r.w {
			c.text(x, y, r.x+r.w-x, row.Info, style)
		}
	}
}
func (u *tui) drawFields(c *canvas, r rect) {
	u.setTop(1, u.fieldTop)
	u.drawScrollbar(c, r, 1)
	r.w -= 2
	indices := u.fieldIndices()
	if len(indices) == 0 {
		c.text(r.x, r.y+1, r.w, "Select a packet to inspect its fields.", inkMuted)
		return
	}
	u.fieldSelection = clamp(u.fieldSelection, 0, len(indices)-1)
	for i := 0; i < r.h; i++ {
		at := u.fieldTop + i
		if at >= len(indices) {
			break
		}
		f := u.fields.fields[indices[at]]
		prefix := "  "
		if f.branch {
			prefix = "── "
		}
		style := inkText
		if f.branch {
			style = inkCyan
		}
		if (u.pane == 1 || u.hexEnd > u.hexStart) && at == u.fieldSelection {
			style = inkSelected
			c.fill(r.x, r.y+i, r.w, style)
		}
		c.text(r.x, r.y+i, r.w, prefix+f.text, style)
	}
}
func (u *tui) drawBytes(c *canvas, r rect) {
	u.setTop(2, u.hexTop)
	u.drawScrollbar(c, r, 2)
	r.w -= 2
	raw := u.inspectedPacket()
	if raw == nil {
		c.text(r.x, r.y+1, r.w, "Raw bytes appear immediately on selection.", inkMuted)
		return
	}
	perRow := bytesPerRow(r.w)
	rows := (len(raw.data) + perRow - 1) / perRow
	u.hexTop = clamp(u.hexTop, 0, max(0, rows-r.h))
	for y := 0; y < r.h; y++ {
		offset := (u.hexTop + y) * perRow
		if offset >= len(raw.data) {
			break
		}
		c.put(r.x, r.y+y, fmt.Sprintf("%06x", offset), inkMuted)
		asciiX := r.x + 9 + perRow*3 + (perRow-1)/8
		for i := 0; i < perRow && offset+i < len(raw.data); i++ {
			b := raw.data[offset+i]
			style := inkBlue
			if b == 0 {
				style = inkMuted
			} else if b >= 32 && b < 127 {
				style = inkGreen
			}
			if offset+i >= u.hexStart && offset+i < u.hexEnd {
				style = inkSelected
			}
			c.put(r.x+8+i*3+i/8, r.y+y, fmt.Sprintf("%02x", b), style)
			ch := "·"
			if b >= 32 && b < 127 {
				ch = string(b)
			}
			c.put(asciiX+i, r.y+y, ch, style)
		}
	}
}

func (u *tui) drawScrollbar(c *canvas, r rect, pane int) {
	total := u.rowCount(pane)
	if total <= r.h || r.h <= 0 {
		return
	}
	start, size := scrollbarGeometry(u.top(pane), total, r.h)
	for i := 0; i < r.h; i++ {
		glyph, style := "│", inkBorder
		if i >= start && i < start+size {
			glyph = "┃"
			style = inkBlue
			if u.pane == pane {
				style = inkCyan
			}
		}
		c.put(r.x+r.w-1, r.y+i, glyph, style)
	}
}
