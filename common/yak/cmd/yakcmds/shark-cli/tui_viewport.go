package sharkcli

type scrollbarDrag struct {
	active     bool
	pane, grab int
}

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }
func panelContent(r rect) rect        { return rect{r.x + 2, r.y + 1, r.w - 4, r.h - 2} }
func (u *tui) panel(pane int) (rect, bool) {
	if u.width < 70 || u.height < 24 {
		return rect{}, false
	}
	l := layoutDashboard(u.width-1, u.height)
	switch pane {
	case 0:
		return l.packets, true
	case 1:
		return l.fields, u.pane != 3 && (l.split || u.pane != 2)
	case 2:
		return l.bytes, u.pane != 3 && (l.split || u.pane == 2)
	case 3:
		return l.stream, u.pane == 3
	}
	return rect{}, false
}
func (u *tui) viewport(pane int) rect {
	r, _ := u.panel(pane)
	r = panelContent(r)
	if pane == 0 {
		r.y++
		r.h--
	}
	return r
}
func (u *tui) rowCount(pane int) int {
	switch pane {
	case 0:
		return len(u.visible)
	case 1:
		return len(u.fieldIndices())
	case 2:
		if raw := u.inspectedPacket(); raw != nil {
			return (len(raw.data) + u.hexPerRow() - 1) / u.hexPerRow()
		}
	case 3:
		return len(u.streamLines())
	}
	return 0
}
func (u *tui) inspectedPacket() *capturedPacket {
	if u.inspect != nil {
		return u.inspect
	}
	if u.selection >= 0 && u.selection < len(u.visible) {
		return u.visible[u.selection].packet
	}
	return nil
}
func bytesPerRow(width int) int {
	if width >= 145 {
		return 32
	}
	if width >= 77 {
		return 16
	}
	return 8
}
func (u *tui) hexPerRow() int {
	// One column is reserved for the scroll thumb, one for a visual gutter.
	return bytesPerRow(u.viewport(2).w - 2)
}
func (u *tui) top(pane int) int {
	switch pane {
	case 0:
		return u.listTop
	case 1:
		return u.fieldTop
	case 2:
		return u.hexTop
	case 3:
		return u.streamTop
	}
	return 0
}
func (u *tui) setTop(pane, top int) {
	top = clamp(top, 0, max(0, u.rowCount(pane)-max(1, u.viewport(pane).h)))
	switch pane {
	case 0:
		u.listTop = top
	case 1:
		u.fieldTop = top
	case 2:
		u.hexTop = top
	case 3:
		u.streamTop = top
	}
}
func (u *tui) ensureSelectionVisible(pane int) {
	selected := u.selection
	if pane == 1 {
		selected = u.fieldSelection
	}
	if pane >= 2 || selected < 0 {
		return
	}
	top, rows := u.top(pane), max(1, u.viewport(pane).h)
	if selected < top {
		top = selected
	}
	if selected >= top+rows {
		top = selected - rows + 1
	}
	u.setTop(pane, top)
}

// Browsing owns a bounded snapshot, independent of the live ring's reused
// slots. Neither new arrivals nor eviction can move rows underneath a pointer.
func (u *tui) freezeList() {
	if u.frozen == nil {
		u.frozen = make([]packetEntry, u.ring.size)
		for i := range u.frozen {
			u.frozen[i] = *u.ring.at(i)
		}
		u.frozenAt = u.ring.version
		u.filterDirty = true
	}
	u.follow = false
	u.refreshVisible()
}
func (u *tui) resumeLive() {
	u.frozen = nil
	u.follow = true
	u.pane = 0
	u.filterDirty = true
	u.drag = scrollbarDrag{}
	u.refreshVisible()
	u.ensureSelectionVisible(0)
}
func (u *tui) focusPane(pane int) {
	u.freezeList()
	u.pane = pane
	if pane == 3 {
		u.syncStream()
	}
}
func (u *tui) scroll(pane, delta int) {
	if pane == 3 {
		u.streamPinned = true
	}
	u.focusPane(pane)
	if pane == 1 {
		u.touchFields()
	}
	u.setTop(pane, u.top(pane)+delta)
	// Scrolling changes a viewport, not the inspected packet or selected field.
}
func (u *tui) resize(width, height int) {
	oldColumns := u.hexPerRow()
	byteOffset := u.hexTop * oldColumns
	u.width = width
	u.height = height
	u.drag = scrollbarDrag{}
	u.hexTop = byteOffset / u.hexPerRow()
	for pane := 0; pane < 4; pane++ {
		u.setTop(pane, u.top(pane))
	}
}

func scrollbarGeometry(top, total, rows int) (start, size int) {
	rows = max(1, rows)
	if total <= rows {
		return 0, rows
	}
	size = max(1, rows*rows/total)
	start = clamp(top, 0, total-rows) * (rows - size) / (total - rows)
	return start, size
}
func (u *tui) dragScrollbar(y int) {
	pane := u.drag.pane
	r := u.viewport(pane)
	_, size := scrollbarGeometry(u.top(pane), u.rowCount(pane), r.h)
	travel := r.h - size
	if travel <= 0 {
		u.setTop(pane, 0)
		return
	}
	offset := clamp(y-r.y-u.drag.grab, 0, travel)
	maximum := max(0, u.rowCount(pane)-r.h)
	u.setTop(pane, (offset*maximum+travel/2)/travel)
}
func (u *tui) mouse(m mouseEvent) {
	if m.action == mouseRelease {
		u.drag = scrollbarDrag{}
		return
	}
	if m.action == mouseMotion {
		if u.drag.active && m.button == 0 {
			u.dragScrollbar(m.y)
		}
		return
	}
	if m.action == mousePress {
		u.drag = scrollbarDrag{}
		if u.searchClick(m) {
			return
		}
		if m.button == 0 && u.width >= 70 && u.height >= 24 {
			l := layoutDashboard(u.width-1, u.height)
			if m.y == l.tabs {
				if u.pane == 3 {
					for i, x := range []int{54, 71, 85} {
						length := []int{13, 10, 12}[i]
						if x+length < u.width-1 && m.x >= x && m.x < x+length {
							u.key([]string{"h", "l", "e"}[i], nil)
							return
						}
					}
				}
				for i, x := range []int{3, 20, 36} {
					if m.x >= x && m.x < x+14 {
						u.focusPane(i + 1)
						return
					}
				}
			}
		}
	}
	pane := -1
	for i := 0; i < 4; i++ {
		if r, shown := u.panel(i); shown && r.contains(m.x, m.y) {
			pane = i
			break
		}
	}
	if pane < 0 {
		return
	}
	if m.action == mouseWheel {
		// Terminals report trackpad horizontal movement separately; consume it
		// without treating the report's trailing bytes as keyboard shortcuts.
		if !m.horizontal && !m.shift {
			u.scroll(pane, m.delta)
		}
		return
	}
	if m.action != mousePress || m.button != 0 {
		return
	}
	u.focusPane(pane)
	if pane == 1 {
		u.touchFields()
	}
	r := u.viewport(pane)
	if !r.contains(m.x, m.y) {
		return
	}
	if m.x == r.x+r.w-1 {
		if pane == 3 {
			u.streamPinned = true
		}
		start, size := scrollbarGeometry(u.top(pane), u.rowCount(pane), r.h)
		grab := m.y - r.y - start
		if grab < 0 || grab >= size {
			grab = size / 2
		}
		u.drag = scrollbarDrag{active: true, pane: pane, grab: grab}
		u.dragScrollbar(m.y)
		return
	}
	index := u.top(pane) + m.y - r.y
	switch pane {
	case 0:
		if index < len(u.visible) {
			u.selection = index
			u.selected = u.visible[index].packet.number
		}
	case 1:
		u.touchFields()
		indices := u.fieldIndices()
		if index < len(indices) {
			u.fieldSelection = index
			u.selectFieldBytes()
		}
	case 2:
		u.selectByteAt(m.x, m.y)
	}
}
func (u *tui) event(event terminalEvent, src *captureSource) bool {
	if event.focus != 0 {
		u.drag = scrollbarDrag{}
		return false
	}
	if event.mouse != nil {
		u.mouse(*event.mouse)
		return false
	}
	if event.paste != "" {
		if u.editor != "" {
			// Paste remains data, including q, Enter and escape characters.
			text := safeText(event.paste)
			available := max(0, 4096-len(u.input))
			for _, r := range text {
				if len(string(r)) > available {
					break
				}
				u.input += string(r)
				available -= len(string(r))
			}
		}
		return false
	}
	return u.key(event.key, src)
}

func (u *tui) applyDetail(result detailResult) {
	if result.streamID != 0 {
		verified := false
		if result.err == nil {
			verified = u.streams.identify(result.streamID, result.detail.protocol)
		}
		if u.stream != nil && u.stream.id == result.streamID && u.stream.decodeVersion == result.streamVersion {
			u.streamFields = result.detail
			u.streamStatus = "Stream decode ready · d fields · t text · x hex · r refresh"
			if result.err != nil {
				u.streamStatus = "Stream decoder unavailable; reassembled bytes remain available"
			}
			if verified {
				u.stream.protocol, u.stream.evidence = u.streams.label(result.streamID)
			}
			u.streamRows = nil
		}
		return
	}
	if u.follow || result.number != u.selected {
		return
	}
	if result.err != nil {
		u.decodeStatus = "Deep decode unavailable · using packet headers"
		return
	}
	// Once the user has interacted with the fields, keep their viewport and
	// structure stable. Cache the full result until an explicit 'd' requests it.
	if u.fieldsTouched {
		u.pendingDetail = &result.detail
		u.decodeStatus = "Deep decode ready · d opens all fields"
		return
	}
	u.installDetail(result.detail)
}
func (u *tui) installDetail(detail packetDetail) {
	u.fields = detail
	u.fieldTop = 0
	u.fieldSelection = 0
	u.hexStart, u.hexEnd = 0, 0
	u.pendingDetail = nil
	u.fieldsTouched = false
	u.decodeStatus = "Deep decode complete"
}
func (u *tui) touchFields() { u.fieldsTouched = true }

// Cursor movement is distinct from wheel scrolling: keys intentionally reveal
// the new selected row, while a wheel leaves selection and detail unchanged.
func (u *tui) move(delta int) {
	u.freezeList()
	pane := u.pane
	if pane >= 2 {
		if pane == 3 {
			u.streamPinned = true
		}
		u.setTop(pane, u.top(pane)+delta)
		return
	}
	rows := max(1, u.viewport(pane).h)
	top := u.top(pane)
	selected := u.selection
	total := len(u.visible)
	if pane == 1 {
		u.touchFields()
		selected = u.fieldSelection
		total = len(u.fieldIndices())
	}
	if total == 0 {
		return
	}
	if selected < top || selected >= top+rows {
		selected = top
		if delta < 0 {
			selected = min(total-1, top+rows-1)
		}
	} else {
		selected += delta
	}
	selected = clamp(selected, 0, total-1)
	if pane == 0 {
		u.selection = selected
		u.selected = u.visible[selected].packet.number
	} else {
		u.fieldSelection = selected
		u.selectFieldBytes()
	}
	u.ensureSelectionVisible(pane)
}

func (u *tui) boundary(last bool) {
	u.freezeList()
	at := 0
	if last {
		at = max(0, u.rowCount(u.pane)-1)
	}
	switch u.pane {
	case 0:
		if len(u.visible) > 0 {
			u.selection = at
			u.selected = u.visible[at].packet.number
			u.ensureSelectionVisible(0)
		}
	case 1:
		u.touchFields()
		u.fieldSelection = at
		u.selectFieldBytes()
		u.ensureSelectionVisible(1)
	case 2:
		u.setTop(2, at)
	case 3:
		u.streamPinned = true
		u.setTop(3, at)
	}
}
