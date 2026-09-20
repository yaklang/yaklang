package sharkcli

import (
	"fmt"
	"strconv"
	"strings"
)

const defaultPacketCapacity = 20000

func (u *tui) openSearch() {
	u.editor, u.input = "Search", u.searchText
	u.notice = "proto:http ip:10.223 content:hello hex:00ff stream:hello · AND / OR / ! · Enter apply"
}
func (u *tui) openCache() {
	u.editor, u.input = "Cache", strconv.Itoa(len(u.ring.entries))
	u.notice = "Packet cache: 1–100000 · 20000 / 20k / 2w · shrinking drops oldest cached packets"
}
func (u *tui) applySearch(input string) error {
	q, err := compileSearch(input)
	if err != nil {
		return err
	}
	u.search, u.searchText = q, strings.TrimSpace(input)
	u.searchGeneration++
	u.filterDirty = true
	u.selected, u.inspect = 0, nil
	u.listTop = 0
	u.fieldTop, u.fieldSelection, u.hexTop = 0, 0, 0
	u.hexStart, u.hexEnd = 0, 0
	return nil
}

func (r *packetRing) resize(capacity int) {
	keep := min(capacity, r.size)
	entries := make([]packetEntry, capacity)
	for i := 0; i < keep; i++ {
		entries[i] = *r.at(r.size - keep + i)
	}
	r.evicted += uint64(r.size - keep)
	r.entries, r.start, r.size = entries, 0, keep
	r.version++
}
func (u *tui) resizePacketCache(input string) error {
	value := strings.ToLower(strings.TrimSpace(input))
	multiplier := 1
	for _, suffix := range []struct {
		name   string
		factor int
	}{{"k", 1000}, {"w", 10000}, {"万", 10000}} {
		if strings.HasSuffix(value, suffix.name) {
			value = strings.TrimSuffix(value, suffix.name)
			multiplier = suffix.factor
			break
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 100000/multiplier {
		return fmt.Errorf("use 1–100000 packets, e.g. 20000, 20k or 2w")
	}
	n *= multiplier
	if n != len(u.ring.entries) {
		u.ring.resize(n)
		if len(u.frozen) > n {
			u.frozen = append([]packetEntry(nil), u.frozen[len(u.frozen)-n:]...)
			u.frozenAt++
		}
		u.visible = nil
		u.filterDirty = true
		u.refreshVisible()
		u.ensureSelectionVisible(0)
	}
	u.notice = fmt.Sprintf("Cache capacity: %d packets · keeps newest packets; increasing cannot recover evicted history", n)
	return nil
}

func searchControls(width int) (input, clear, cache rect) {
	// This occupies the formerly blank second terminal row, so pane viewports
	// retain their geometry and all mouse coordinates share the same layout.
	return rect{2, 1, max(1, width-34), 1}, rect{width - 31, 1, 7, 1}, rect{width - 22, 1, 20, 1}
}
func (u *tui) drawSearch(c *canvas) {
	input, clear, cache := searchControls(c.width)
	text := "[/ Search] proto:http ip:10.223 content:hello"
	style := inkMuted
	if u.searchText != "" {
		text, style = "[/ Search] "+u.searchText, inkCyan
	}
	if u.editor == "Search" {
		text, style = "[/ Search] "+u.input+"▏", inkSelected
	}
	c.fill(input.x, input.y, input.w, inkHeader)
	c.text(input.x, input.y, input.w, text, style)
	c.text(clear.x, clear.y, clear.w, "[Clear]", inkMuted)
	c.text(cache.x, cache.y, cache.w, fmt.Sprintf("[c Cache %d]", len(u.ring.entries)), inkCyan)
}

func (u *tui) searchClick(m mouseEvent) bool {
	if m.action != mousePress || m.button != 0 || u.width < 70 || u.height < 24 {
		return false
	}
	input, clear, cache := searchControls(u.width - 1)
	switch {
	case input.contains(m.x, m.y):
		u.openSearch()
	case clear.contains(m.x, m.y):
		_ = u.applySearch("")
		u.editor, u.input, u.notice = "", "", "Search cleared; BPF display filter retained"
	case cache.contains(m.x, m.y):
		u.openCache()
	default:
		return false
	}
	return true
}
