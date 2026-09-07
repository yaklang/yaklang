package base

// A private log entry is both the current value and (when replay is true) a
// historical assignment. Overwriting appends a version instead of allocating
// a second key/value journal. Nothing here is shared between configurations.
type compactConfigWrite struct {
	value  any
	key    uint8
	replay bool
}

var compactConfigKeys = [...]string{
	"", CfgOptionFuns, "endian", "parser", "unit", "isTerminal", "type", "list",
	"node result", "parent", "length", "element index", "last node", "isList",
	"isRoot", "package-child", "operator", "out", "import", "ref-type", "is ref type",
	"consumed bits", "delimiter", "del", "delimiter-optional", "length-from-field",
	"length-for-field", "length-for-start-field", "inList", "temp root", "stop-value", "exception-plan",
}

// Keep this a compile-time string switch: there is no global registry, lock,
// unbounded interning or map lookup per field. Unknown keys use the old store.
func compactConfigKey(key string) uint8 {
	switch key {
	case CfgOptionFuns:
		return 1
	case "endian":
		return 2
	case "parser":
		return 3
	case "unit":
		return 4
	case "isTerminal":
		return 5
	case "type":
		return 6
	case "list":
		return 7
	case "node result":
		return 8
	case "parent":
		return 9
	case "length":
		return 10
	case "element index":
		return 11
	case "last node":
		return 12
	case "isList":
		return 13
	case "isRoot":
		return 14
	case "package-child":
		return 15
	case "operator":
		return 16
	case "out":
		return 17
	case "import":
		return 18
	case "ref-type":
		return 19
	case "is ref type":
		return 20
	case "consumed bits":
		return 21
	case "delimiter":
		return 22
	case "del":
		return 23
	case "delimiter-optional":
		return 24
	case "length-from-field":
		return 25
	case "length-for-field":
		return 26
	case "length-for-start-field":
		return 27
	case "inList":
		return 28
	case "temp root":
		return 29
	case "stop-value":
		return 30
	case "exception-plan":
		return 31
	}
	return 0
}

func (s *configStore) compactGet(key string) (any, bool) {
	k := compactConfigKey(key)
	if k == 0 || s.positions[k] == 0 {
		return nil, false
	}
	// Has uses presence only; Get(CfgOptionFuns) expands and exposes callbacks.
	if k == 1 {
		return nil, true
	}
	return s.writes[s.positions[k]-1].value, true
}

func (s *configStore) compactSet(key string, value any, replay bool, reserve int) bool {
	if s.configStoreLegacy != nil {
		return false
	}
	k := compactConfigKey(key)
	if k <= 1 || len(s.writes) >= 65534 {
		return false
	}
	if s.writes == nil {
		s.writes = make([]compactConfigWrite, 0, reserve)
	}
	s.writes = append(s.writes, compactConfigWrite{value, k, replay})
	if s.positions[k] == 0 {
		s.order[s.orderLen] = k
		s.orderLen++
	}
	s.positions[k] = uint16(len(s.writes))
	if replay {
		s.historyCount++
		if s.positions[1] == 0 {
			s.order[s.orderLen] = 1
			s.orderLen++
			s.positions[1] = 1
		}
	}
	return true
}

// Expansion preserves current insertion order, historical values and shallow
// aliases. It does not expose callbacks: ordinary replay can still use data.
// Caller must hold the write lock (or own an unpublished configuration).
func (s *configStore) expandLocked() {
	if s.configStoreLegacy != nil {
		return
	}
	legacy := &configStoreLegacy{}
	if s.orderLen <= 8 {
		legacy.entries = legacy.inline[:0]
	} else {
		legacy.entries = make([]configEntry, 0, int(s.orderLen))
	}
	for _, k := range s.order[:s.orderLen] {
		var value any
		if k == 1 {
			journal := &configReplay{writes: make([]configEntry, 0, int(s.historyCount))}
			for _, w := range s.writes {
				if w.replay {
					journal.writes = append(journal.writes, configEntry{compactConfigKeys[w.key], w.value})
				}
			}
			value = journal
		} else {
			value = s.writes[s.positions[k]-1].value
		}
		legacy.entries = append(legacy.entries, configEntry{compactConfigKeys[k], value})
	}
	if len(legacy.entries) > 16 {
		legacy.index = make(map[string]int, len(legacy.entries))
		for i, e := range legacy.entries {
			legacy.index[e.key] = i
		}
	}
	s.configStoreLegacy = legacy
	s.writes = nil
	s.positions = [32]uint16{}
	s.order = [32]uint8{}
	s.orderLen = 0
	s.historyCount = 0
}
