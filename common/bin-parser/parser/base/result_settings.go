package base

// ResultSettings is a fresh snapshot of the configuration used to project a
// node. It does not cache values across projections or expose the private store.
// Position intentionally retains its public dynamic type: malformed spans
// follow the existing assertion/error path at the point they are consumed.
type ResultSettings struct {
	Position       any
	Present        bool
	Type, Endian   string
	Terminal, List bool
}

func (c *Config) ResultSettings() (r ResultSettings) {
	s := c.data
	if s == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	get := func(key uint8) (any, bool) {
		if s.configStoreLegacy == nil {
			position := s.positions[key]
			if position == 0 {
				return nil, false
			}
			return s.compactWrite(int(position - 1)).value, true
		}
		if i, ok := s.findLocked(compactConfigKeys[key]); ok {
			return s.entries[i].value, true
		}
		return nil, false
	}
	// These IDs are private to the compact store; "list" is distinct from
	// the legacy descriptor flag "isList".
	r.Position, r.Present = get(8)
	v, _ := get(6)
	r.Type, _ = v.(string)
	v, _ = get(2)
	r.Endian, _ = v.(string)
	v, _ = get(5)
	r.Terminal, _ = v.(bool)
	v, _ = get(7)
	r.List, _ = v.(bool)
	return
}
