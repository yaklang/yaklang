package stream_parser

import "bytes"

// Field descriptions are private inputs to the result-tree builder. Public
// Nodes, metadata, strings and byte values never borrow this storage.
const fieldArenaRetainedFields = 4096

var idleFieldArenas = make(chan *fieldArena, 16)

type fieldArena struct {
	blocks []fieldArenaBlock
	block  int
	// Fresh backing storage for result bytes, never returned to the idle pool.
	outputBytes     []byte
	outputReserve   int
	structuredValue any
	values          []structuredFieldValue
	valuesUsed      int
}

func (a *fieldArena) appendValue(values []structuredFieldValue, name string, value any) []structuredFieldValue {
	values = append(values, structuredFieldValue{name: name, value: value})
	a.values = values[:cap(values)]
	a.valuesUsed = max(a.valuesUsed, len(values))
	return values
}

func (a *fieldArena) cloneBytes(value []byte) []byte {
	if a == nil || a.outputReserve == 0 || len(value) == 0 {
		return bytes.Clone(value)
	}
	if len(a.outputBytes) < len(value) {
		a.outputBytes = make([]byte, max(a.outputReserve, len(value)))
	}
	owned := a.outputBytes[:len(value):len(value)]
	copy(owned, value)
	a.outputBytes = a.outputBytes[len(value):]
	return owned
}

type fieldArenaBlock struct {
	fields []tlsCertificateField
	used   int
}

func acquireFieldArena() *fieldArena {
	select {
	case a := <-idleFieldArenas:
		return a
	default:
		return &fieldArena{}
	}
}

func (a *fieldArena) allocate(n int) []tlsCertificateField {
	if n == 0 {
		return nil
	}
	if a == nil {
		return make([]tlsCertificateField, n)
	}
	for a.block < len(a.blocks) {
		block := &a.blocks[a.block]
		if len(block.fields)-block.used >= n {
			start := block.used
			block.used += n
			return block.fields[start:block.used:block.used]
		}
		a.block++
	}
	block := make([]tlsCertificateField, max(256, n))
	a.blocks = append(a.blocks, fieldArenaBlock{fields: block, used: n})
	return block[:n:n]
}

func (a *fieldArena) release() {
	a.outputBytes = nil
	a.outputReserve = 0
	a.structuredValue = nil
	total := len(a.values)
	for _, block := range a.blocks {
		total += len(block.fields)
	}
	if total > fieldArenaRetainedFields {
		return // Large or adversarial inputs do not enlarge the retained pool.
	}
	clear(a.values[:a.valuesUsed])
	a.valuesUsed = 0
	for i := range a.blocks {
		block := &a.blocks[i]
		clear(block.fields[:block.used]) // Drop every reference written by this lease.
		block.used = 0
	}
	a.block = 0
	select {
	case idleFieldArenas <- a:
	default:
	}
}
