package base

// LookupConsumedResult reads the current result and its optional wire-length
// override together. It never caches a subtree or loses present-nil values.
// A consumed length is relevant only when this node has an observed result.
func (c *Config) LookupConsumedResult() (result, consumed any, hasResult, hasConsumed bool) {
	s := c.data
	if s == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.configStoreLegacy == nil {
		position := s.positions[8] // node result
		if position == 0 {
			return
		}
		result, hasResult = s.compactWrite(int(position-1)).value, true
		if position = s.positions[21]; position != 0 { // consumed bits
			consumed, hasConsumed = s.compactWrite(int(position-1)).value, true
		}
		return
	}
	i, ok := s.findLocked(CfgNodeResult)
	if !ok {
		return
	}
	result, hasResult = s.entries[i].value, true
	if i, ok = s.findLocked("consumed bits"); ok {
		consumed, hasConsumed = s.entries[i].value, true
	}
	return
}
