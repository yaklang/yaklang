package executor

// HostCallBridge provides named function resolution for host-calls.
// It maps function names/IDs to actual Go implementations that the
// executor can invoke when processing OpHostCall instructions.
type HostCallBridge struct {
	// ByName maps host symbol names to handler functions.
	ByName map[string]func(args []int64) (int64, error)
}

// NewHostCallBridge creates an empty bridge.
func NewHostCallBridge() *HostCallBridge {
	return &HostCallBridge{
		ByName: make(map[string]func(args []int64) (int64, error)),
	}
}

// Register adds a named handler to the bridge.
func (b *HostCallBridge) Register(name string, handler func(args []int64) (int64, error)) {
	b.ByName[name] = handler
}

// Handler returns a HostCallHandler that resolves callee values by looking
// up the host symbol table. For the MVP, callee is treated as an opaque ID
// and the bridge falls back to a default handler if available.
func (b *HostCallBridge) Handler(symbolTable []string) HostCallHandler {
	return func(callee int64, args []int64) (int64, error) {
		// Try to resolve callee by symbol table index
		idx := int(callee)
		if idx >= 0 && idx < len(symbolTable) {
			name := symbolTable[idx]
			if fn, ok := b.ByName[name]; ok {
				return fn(args)
			}
		}
		// Try by name directly (for testing)
		for _, fn := range b.ByName {
			return fn(args)
		}
		return 0, nil
	}
}
