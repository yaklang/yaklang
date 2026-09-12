package base

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ParserFactory builds the mutable parser runtime used by one node tree.
// RegisterParser remains available for parsers whose implementation is safe to
// share, while stateful parsers can opt in to per-tree construction.
type ParserFactory func() Parser

// ParserRegistration returns the registered parser or factory identity. Like
// RegisterParser, this is a configuration-time registry, not a concurrent
// replacement API. A factory registration must not be invoked as a parser.
func ParserRegistration(name string) Parser {
	return parseMap[name]
}

// parserFactoryRegistration occupies the existing parser registry so a later
// RegisterParser call keeps its historical replacement semantics. The embedded
// Parser is never invoked directly: Node.getParser resolves the registration to
// a concrete runtime first.
type parserFactoryRegistration struct {
	Parser
	factory ParserFactory
}

// parserRuntimeMap is stored in the logical root's NodeContext. Imported rule
// nodes can have a different context, so Node.getParser resolves their outermost
// parent before consulting this map.
type parserRuntimeMap struct {
	mu      sync.Mutex
	parsers map[string]parserRuntimeEntry
	cached  atomic.Pointer[parserRuntimeLookup]
}

// Most trees repeatedly resolve the same parser. Publish one immutable lookup
// entry so established runtimes need no map lock on each field/result access.
// The registration identity is checked on every read, including replacement.
type parserRuntimeLookup struct {
	name string
	parserRuntimeEntry
}

type parserRuntimeEntry struct {
	registration *parserFactoryRegistration
	parser       Parser
}

const ctxParserRuntimeMap = "__bin_parser_runtime_parsers"

func newParserRuntimeMap() *parserRuntimeMap {
	return &parserRuntimeMap{parsers: make(map[string]parserRuntimeEntry)}
}

// RegisterParserFactory registers a stateful parser without changing the
// singleton behavior of RegisterParser. Registering a concrete parser under the
// same name later replaces this factory exactly as it did before factories
// existed.
func RegisterParserFactory(name string, factory ParserFactory) {
	RegisterParser(name, &parserFactoryRegistration{factory: factory})
}

func (r *parserRuntimeMap) parser(name string, registration *parserFactoryRegistration) (Parser, error) {
	if cached := r.cached.Load(); cached != nil && cached.name == name && cached.registration == registration {
		return cached.parser, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry := r.parsers[name]; entry.registration == registration && entry.parser != nil {
		r.cacheLocked(name, entry)
		return entry.parser, nil
	}
	if registration.factory == nil {
		return nil, fmt.Errorf("parser %s has a nil factory", name)
	}
	parser := registration.factory()
	if parser == nil {
		return nil, fmt.Errorf("parser %s factory returned nil", name)
	}
	entry := parserRuntimeEntry{registration: registration, parser: parser}
	r.parsers[name] = entry
	r.cacheLocked(name, entry)
	return parser, nil
}

func (r *parserRuntimeMap) load(name string, registration *parserFactoryRegistration) Parser {
	if cached := r.cached.Load(); cached != nil && cached.name == name && cached.registration == registration {
		return cached.parser
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.parsers[name]
	if entry.registration != registration {
		return nil
	}
	if entry.parser != nil {
		r.cacheLocked(name, entry)
	}
	return entry.parser
}

func (r *parserRuntimeMap) bind(name string, registration *parserFactoryRegistration, parser Parser) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.parsers[name]
	if entry.registration != registration || entry.parser == nil {
		entry = parserRuntimeEntry{registration: registration, parser: parser}
		r.parsers[name] = entry
	}
	if entry.parser != nil {
		r.cacheLocked(name, entry)
	}
}

func (r *parserRuntimeMap) cacheLocked(name string, entry parserRuntimeEntry) {
	if cached := r.cached.Load(); cached != nil {
		// Keep the first established name hot. Alternating custom parsers must
		// not allocate a replacement cache record on every lookup.
		if cached.name != name || cached.registration == entry.registration {
			return
		}
	}
	r.cached.Store(&parserRuntimeLookup{name: name, parserRuntimeEntry: entry})
}
