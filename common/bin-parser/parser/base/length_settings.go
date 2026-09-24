package base

// LengthSettings reads the current scalar length configuration with one
// shared lock. No node pointers, derived boundaries or packet values are
// cached across operations. Presence is separate from a zero/invalid value.
type LengthSettings struct {
	Length                       uint64
	Type, Field                  string
	HasLength, HasType, HasField bool
}

func (c *Config) LengthSettings() (r LengthSettings) {
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
	v, present := get(10)
	r.HasLength = present
	if present {
		r.Length, _ = InterfaceToUint64(v)
		return
	}
	v, r.HasType = get(6)
	r.Type, _ = v.(string)
	v, r.HasField = get(25)
	r.Field, _ = v.(string)
	return
}
