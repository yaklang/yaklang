package base

import (
	"fmt"
	"sync"
)

// ParserFactory builds the mutable parser runtime used by one node tree.
// RegisterParser remains available for parsers whose implementation is safe to
// share, while stateful parsers can opt in to per-tree construction.
type ParserFactory func() Parser

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
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry := r.parsers[name]; entry.registration == registration && entry.parser != nil {
		return entry.parser, nil
	}
	if registration.factory == nil {
		return nil, fmt.Errorf("parser %s has a nil factory", name)
	}
	parser := registration.factory()
	if parser == nil {
		return nil, fmt.Errorf("parser %s factory returned nil", name)
	}
	r.parsers[name] = parserRuntimeEntry{registration: registration, parser: parser}
	return parser, nil
}

func (r *parserRuntimeMap) load(name string, registration *parserFactoryRegistration) Parser {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.parsers[name]
	if entry.registration != registration {
		return nil
	}
	return entry.parser
}

func (r *parserRuntimeMap) bind(name string, registration *parserFactoryRegistration, parser Parser) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.parsers[name]
	if entry.registration != registration || entry.parser == nil {
		r.parsers[name] = parserRuntimeEntry{registration: registration, parser: parser}
	}
}
