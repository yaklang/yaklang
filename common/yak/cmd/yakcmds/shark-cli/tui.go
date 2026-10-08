package sharkcli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/yaklang/pcap"
	"golang.org/x/term"
)

type packetEntry struct {
	packet                          *capturedPacket
	summary                         packetSummary
	searchGeneration, searchVersion uint64
	searchMatched                   bool
}
type packetRing struct {
	entries     []packetEntry
	start, size int
	evicted     uint64
	version     uint64
}

func newPacketRing(capacity int) packetRing {
	return packetRing{entries: make([]packetEntry, capacity)}
}
func (r *packetRing) add(p *capturedPacket) {
	at := (r.start + r.size) % len(r.entries)
	if r.size == len(r.entries) {
		at = r.start
		r.start = (r.start + 1) % len(r.entries)
		r.evicted++
	} else {
		r.size++
	}
	r.entries[at] = packetEntry{packet: p}
	r.version++
}
func (r *packetRing) at(i int) *packetEntry { return &r.entries[(r.start+i)%len(r.entries)] }

func (e *packetEntry) row() packetSummary {
	if e.summary.Number == 0 {
		e.summary = summarize(e.packet)
	}
	return e.summary
}

type tui struct {
	streamBeginning                                     bool
	streamPinned                                        bool
	streamRefresh                                       time.Time
	streams                                             *streamStore
	stream                                              *streamSnapshot
	streamPacket                                        uint64
	streamTop, streamMode, streamDirection, streamWidth int
	streamRows                                          []streamLine
	streamFields                                        packetDetail
	streamStatus                                        string
	frozen                                              []packetEntry
	frozenAt                                            uint64
	drag                                                scrollbarDrag
	fieldsTouched                                       bool
	pendingDetail                                       *packetDetail

	inspect           *capturedPacket
	inspectionChanged time.Time
	decodeStatus      string
	visibleVersion    uint64
	filterDirty       bool
	rate              trafficRate
	mix               []protocolShare
	mixTotal          int

	ring                             packetRing
	visible                          []*packetEntry
	selected                         uint64
	selection, listTop               int
	follow                           bool
	pane                             int
	fields                           packetDetail
	fieldSelection, fieldTop, hexTop int
	hexStart, hexEnd                 int
	displayFilter                    *pcap.BPF
	filterText                       string
	searchText                       string
	search                           *packetSearch
	searchGeneration                 uint64
	editor, input, notice            string
	width, height                    int
	finished                         bool
	finishErr                        error
	color                            bool
}

type detailRequest struct {
	ctx    context.Context
	packet *capturedPacket
	stream *streamSnapshot
}
type detailResult struct {
	streamID, streamVersion uint64
	number                  uint64
	detail                  packetDetail
	err                     error
}

func runTUI(ctx context.Context, s *captureSession, src *captureSource, cfg captureConfig, capacity int) error {
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), old)
	fmt.Fprint(os.Stdout, terminalEnter)
	defer fmt.Fprint(os.Stdout, terminalLeave)
	ui := &tui{ring: newPacketRing(capacity), follow: !src.offline, width: 100, height: 32, color: os.Getenv("NO_COLOR") == "", streams: s.streams}
	ui.rate.reset(time.Now(), s)
	keyCtx, cancel := context.WithCancel(ctx)
	inputs := make(chan []byte, 64)
	go readTerminalInput(keyCtx, os.Stdin, inputs)
	var decoder inputDecoder
	requests := make(chan detailRequest, 1)
	results := make(chan detailResult, 1)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case <-keyCtx.Done():
				return
			case req := <-requests:
				if req.ctx.Err() != nil {
					continue
				}
				var result detailResult
				if req.stream != nil {
					result.streamID, result.streamVersion = req.stream.id, req.stream.decodeVersion
					result.detail, result.err = decodeStreamIsolated(req.ctx, req.stream)
				} else {
					result.number = req.packet.number
					result.detail, result.err = decodeIsolated(req.ctx, req.packet)
				}
				select {
				case results <- result:
				case <-keyCtx.Done():
					return
				}
			}
		}
	}()
	// Cancel and reap any active decoder before restoring the terminal.
	defer func() { cancel(); <-workerDone }()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	packets := s.packets
	var requested uint64
	var requestedStream, requestedVersion uint64
	var decodeCancel context.CancelFunc
	defer func() {
		if decodeCancel != nil {
			decodeCancel()
		}
	}()
	var lastCapture time.Time
	dirty := false
	screen := screenWriter{out: os.Stdout}
	refresh := func(drain bool) error {
		// One bounded batch per frame. Capture never waits for rendering live data.
		for i := 0; drain && packets != nil && i < cap(s.packets); i++ {
			select {
			case p, ok := <-packets:
				if !ok {
					packets = nil
					ui.finished = true
					ui.finishErr = s.result()
					if ui.finishErr != nil {
						ui.notice = ui.finishErr.Error()
					}
				} else {
					ui.ring.add(p)
				}
			default:
				i = cap(s.packets)
			}
		}
		if w, h, e := term.GetSize(int(os.Stdout.Fd())); e == nil && (w != ui.width || h != ui.height) {
			ui.resize(w, h)
		}
		ui.refreshVisible()
		ui.syncInspection(time.Now())
		ui.syncStream()
		if requested != 0 && (ui.follow || requested != ui.selected || ui.pane == 3) {
			if decodeCancel != nil {
				decodeCancel()
			}
			requested = 0
		}
		if requestedStream != 0 && (ui.pane != 3 || ui.stream == nil || requestedStream != ui.stream.id || requestedVersion != ui.stream.decodeVersion) {
			if decodeCancel != nil {
				decodeCancel()
			}
			requestedStream = 0
		}
		if ui.pane == 3 && ui.stream != nil && ui.stream.decodeVersion != 0 && requestedStream == 0 {
			requestCtx, stop := context.WithCancel(keyCtx)
			select {
			case requests <- detailRequest{ctx: requestCtx, stream: ui.stream}:
				decodeCancel = stop
				requestedStream, requestedVersion = ui.stream.id, ui.stream.decodeVersion
				ui.streamStatus = "Decoding contiguous stream prefixes…"
			default:
				stop()
			}
		}
		if ui.pane != 3 && !ui.follow && ui.inspect != nil && requested != ui.selected && time.Since(ui.inspectionChanged) >= 200*time.Millisecond {
			requestCtx, stop := context.WithCancel(keyCtx)
			select {
			case requests <- detailRequest{ctx: requestCtx, packet: ui.inspect}:
				decodeCancel = stop
				requested = ui.selected
				ui.decodeStatus = "Deep decode running · headers and bytes ready"
			default:
				stop()
			}
		}
		if ui.rate.sample(time.Now(), s) {
			ui.updateMix()
		}
		return screen.draw(ui.view(s, src, cfg), ui.width, ui.height)
	}
	if err := refresh(true); err != nil {
		return err
	}
	lastCapture = time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case d := <-results:
			ui.applyDetail(d)
			dirty = true
		case data, ok := <-inputs:
			if !ok {
				return ui.finishErr
			}
			for _, event := range decoder.feed(data, time.Now()) {
				if ui.event(event, src) {
					return ui.finishErr
				}
			}
			dirty = true
		case now := <-ticker.C:
			for _, event := range decoder.expire(now) {
				if ui.event(event, src) {
					return ui.finishErr
				}
				dirty = true
			}
			drain := now.Sub(lastCapture) >= 200*time.Millisecond
			if dirty || drain {
				if err := refresh(drain); err != nil {
					return err
				}
				dirty = false
				if drain {
					lastCapture = now
				}
			}

		}
	}
}

func (u *tui) syncInspection(now time.Time) {
	var raw *capturedPacket
	if u.selection >= 0 && u.selection < len(u.visible) {
		raw = u.visible[u.selection].packet
	}
	if !u.follow && u.inspect != nil && u.inspect.number == u.selected {
		return
	}
	if raw == nil {
		if u.follow || u.inspect == nil {
			u.inspect = nil
			u.fields = packetDetail{}
			u.hexStart, u.hexEnd = 0, 0
			u.pendingDetail = nil
			u.fieldsTouched = false
		}
		return
	}
	if u.inspect != nil && u.inspect.number == raw.number {
		return
	}
	u.inspect = raw
	u.fieldsTouched = false
	u.pendingDetail = nil
	u.fields = quickDetail(raw)
	u.inspectionChanged = now
	u.fieldSelection = 0
	u.fieldTop = 0
	u.hexTop = 0
	u.hexStart, u.hexEnd = 0, 0
	u.decodeStatus = "Live headers · Enter pins and decodes this packet"
}

func (u *tui) refreshVisible() {
	version := u.ring.version
	if u.frozen != nil {
		version = u.frozenAt
	}
	// A frozen list can acquire a verified protocol/stream payload later. Recheck
	// dynamic predicates while packet-only predicates remain cached per entry.
	if u.visibleVersion != version || u.filterDirty || u.search != nil && u.search.dynamic {
		u.visible = u.visible[:0]
		searchContext := newSearchContext(u.streams)
		size := u.ring.size
		if u.frozen != nil {
			size = len(u.frozen)
		}
		for i := 0; i < size; i++ {
			var e *packetEntry
			if u.frozen != nil {
				e = &u.frozen[i]
			} else {
				e = u.ring.at(i)
			}
			if (u.displayFilter == nil || u.displayFilter.Matches(e.packet.ci, e.packet.data)) && u.matchesSearch(e, searchContext) {
				u.visible = append(u.visible, e)
			}
		}
		u.visibleVersion = version
		u.filterDirty = false
	}
	if len(u.visible) == 0 {
		u.selection = -1
		if u.follow {
			u.selected = 0
		}
		return
	}
	if u.follow {
		u.selection = len(u.visible) - 1
		u.selected = u.visible[u.selection].packet.number
		return
	}
	if u.selected == 0 {
		u.selection = 0
		u.selected = u.visible[0].packet.number
		return
	}
	u.selection = -1
	for i, e := range u.visible {
		if e.packet.number == u.selected {
			u.selection = i
			return
		}
	}
	// Keep an inspected packet alive even after it leaves the rolling cache.
	if u.inspect == nil {
		u.selection = 0
		u.selected = u.visible[0].packet.number
	}
}

func (u *tui) key(key string, src *captureSource) bool {
	if key == "\x1b" {
		if u.editor != "" {
			u.notice = ""
		}
		u.editor = ""
		u.input = ""
		u.drag = scrollbarDrag{}
		return false
	}
	if key == "\x03" {
		return true
	}
	if u.editor != "" {
		switch key {
		case "\r", "\n":
			if u.editor == "Search" {
				if err := u.applySearch(u.input); err != nil {
					u.notice = "Search error: " + err.Error()
					return false
				}
				u.editor, u.input = "", ""
				u.notice = "Search applied · AND between terms · OR / | alternatives · click Clear to reset"
				return false
			}
			if u.editor == "Cache" {
				if err := u.resizePacketCache(u.input); err != nil {
					u.notice = "Cache error: " + err.Error()
					return false
				}
				u.editor, u.input = "", ""
				return false
			}
			expr := u.input
			var err error
			if u.editor == "protocol" {
				expr, err = buildFilter([]string{expr}, "")
			}
			var filter *pcap.BPF
			if err == nil && strings.TrimSpace(expr) != "" {
				filter, err = pcap.NewBPF(src.link, 262144, expr)
			}
			if err != nil {
				u.notice = "Filter error: " + err.Error()
				return false
			}
			u.filterDirty = true
			u.inspect = nil
			u.selected = 0
			u.displayFilter = filter
			u.filterText = expr
			u.editor = ""
			u.input = ""
			u.notice = "Display filter applied; capture/output filter unchanged"
			u.listTop = 0
		case "\x7f", "\b":
			rs := []rune(u.input)
			if len(rs) > 0 {
				u.input = string(rs[:len(rs)-1])
			}
		case "\x15":
			u.input = ""
		default:
			if !strings.HasPrefix(key, "\x1b") && len(u.input) < 4096 {
				for _, r := range key {
					if !unicode.IsControl(r) {
						u.input += string(r)
					}
				}
			}
		}
		return false
	}
	switch key {
	case "1":
		u.pane = 0
	case "2":
		u.focusPane(1)
	case "3":
		u.focusPane(2)
	case "4", "s":
		u.focusPane(3)
	case "q":
		return true
	case "\t":
		u.focusPane((u.pane + 1) % 4)
	case "\x1b[Z":
		u.focusPane((u.pane + 3) % 4)
	case "/", "\x06":
		u.openSearch()
	case "c":
		u.openCache()
	case "f":
		u.editor = "BPF"
		u.input = u.filterText
		u.notice = "Enter applies · Esc cancels · Ctrl+U clears"
	case "p":
		u.editor = "protocol"
		u.input = ""
		u.notice = "Comma-separated: dns,tls,http,tcp,udp,ssh,… · Enter applies"
	case " ":
		if u.follow {
			u.freezeList()
		} else {
			u.resumeLive()
		}
	case "g", "\x1b[H", "\x1b[1~":
		u.boundary(false)
	case "G", "\x1b[F", "\x1b[4~":
		if u.pane == 0 {
			u.resumeLive()
		} else {
			u.boundary(true)
		}
	case "j", "\x1b[B":
		u.move(1)
	case "k", "\x1b[A":
		u.move(-1)
	case "\x1b[6~":
		u.move(max(1, u.viewport(u.pane).h-1))
	case "\x1b[5~":
		u.move(-max(1, u.viewport(u.pane).h-1))
	case "d":
		if u.pane == 3 {
			u.streamPinned = true
			u.streamMode = 2
			u.streamRows = nil
			u.streamTop = 0
		} else if u.pendingDetail != nil {
			u.installDetail(*u.pendingDetail)
		}
	case "x", "t":
		if u.pane == 3 {
			u.streamPinned = true
			u.streamMode = 0
			if key == "x" {
				u.streamMode = 1
			}
			u.streamRows = nil
			u.streamTop = 0
		}
	case "v":
		if u.pane == 3 {
			u.streamPinned = true
			u.streamDirection = (u.streamDirection + 1) % 3
			u.streamRows = nil
			u.streamTop = 0
		}
	case "h", "l", "e":
		if u.pane == 3 {
			u.streamPinned = true
			u.streamTop = 0
			u.streamRows = nil
			if key == "e" {
				u.streamMode = 3
			} else {
				u.streamBeginning = key == "h"
				u.streamMode = 0
			}
		}
	case "r":
		if u.pane == 3 {
			u.loadStream()
		}
	case "\r", "\n":
		if u.pane == 0 {
			u.focusPane(1)
		} else if u.pane == 1 && u.selectFieldBytes() {
			u.focusPane(2)
		}
	}

	return false
}
func (u *tui) fieldIndices() []int {
	indices := make([]int, len(u.fields.fields))
	for i := range indices {
		indices[i] = i
	}
	return indices
}

func clamp(n, low, high int) int { return min(max(n, low), high) }
