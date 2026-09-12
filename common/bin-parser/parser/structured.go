package parser

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// ParseStructured parses one explicitly bounded message into independently
// owned "fields" and "metadata" values, without JSON serialization. The input
// may be reused after return; it must not be modified during the call.
//
// Audited exact embedded native entries can project their existing
// decoder output directly. Other rules, custom parser registrations and native
// failures use ParseBinary and NodeToMap, preserving the original diagnostics.
// The supplied message must be consumed completely. For mutable Nodes, custom
// input configuration or incremental readers, continue to use ParseBinary.
func ParseStructured(data []byte, rule string, keys ...string) (map[string]any, error) {
	if base.ParserRegistration("default") == defaultParserRegistration {
		// Select the rule before checking its fingerprint. Adding an adapter
		// must not add a hash lookup to every unrelated protocol's hot path.
		switch rule {
		case "application-layer.tls_hello":
			if captureStructuredRules()["tls_hello"] && len(keys) == 1 && keys[0] == "TLSClientHello" {
				if value, ok := captureTLSClientHello(data); ok {
					return map[string]any{"fields": value, "metadata": nil}, nil
				}
			}
		case "application-layer.dns":
			if captureStructuredRules()["dns"] && len(keys) == 1 && keys[0] == "DNS" {
				if value, ok := captureDNSStructured(data); ok {
					return value, nil
				}
			}
		case "application-layer.tls":
			if captureStructuredRules()["tls"] && len(keys) == 0 {
				if value, ok := captureTLSStructured(data); ok {
					return value, nil
				}
			}
		case "application-layer.http":
			if captureStructuredRules()["http"] && len(keys) == 1 && keys[0] == "HTTPExact" {
				if value, ok, err := captureHTTPStructured(data, nil); ok && err == nil {
					return value, nil
				}
			}
		}
	}
	if len(keys) == 1 && base.ParserRegistration("default") == defaultParserRegistration {
		var rulePath string
		switch rule {
		case "application-layer.memcached_fields":
			rulePath = "application-layer/memcached_fields.yaml"
		case "application-layer.cassandra_fields":
			rulePath = "application-layer/cassandra_fields.yaml"
		default:
			parts := strings.Split(rule, ".")
			parts[len(parts)-1] += ".yaml"
			rulePath = path.Join(parts...)
		}
		if decoder := structuredDecoder(rulePath, keys[0]); decoder != nil {
			fields, metadata, err := decoder(data)
			if err == nil {
				return map[string]any{"fields": fields, "metadata": metadata}, nil
			}
			// The input is a slice, so replaying a failed decode has no reader
			// side effects. Run the original path for its Yak error and location.
		}
	}
	return parseStructuredFallback(data, rule, keys...)
}

func parseStructuredFallback(data []byte, rule string, keys ...string) (map[string]any, error) {
	reader := &structuredReader{Reader: bytes.NewReader(data), bits: uint64(len(data)) * 8}
	node, err := ParseBinary(reader, rule, keys...)
	if err != nil {
		return nil, err
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("structured parse: %d unconsumed message bytes", reader.Len())
	}
	return map[string]any{"fields": stream_parser.NodeToMap(node), "metadata": node.Cfg.GetItem("additionInfo")}, nil
}

type structuredReader struct {
	*bytes.Reader
	bits uint64
}

func (r *structuredReader) InputBitLength() uint64 { return r.bits }

type structuredRule struct {
	once     sync.Once
	decoders map[string]stream_parser.StructuredDecoder
}

// Only embedded names occupy cache slots. Unknown caller-controlled names can
// never grow the cache. Each rule's positive AND negative decision is compiled
// once, lazily; plans retain neither public Nodes nor per-message state.
var structuredRules = sync.OnceValue(func() map[string]*structuredRule {
	cache := make(map[string]*structuredRule)
	_ = fs.WalkDir(rules.RuleFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(name, ".yaml") {
			cache[name] = &structuredRule{}
		}
		return err
	})
	return cache
})

func structuredDecoder(rulePath, entry string) stream_parser.StructuredDecoder {
	cached := structuredRules()[rulePath]
	if cached == nil {
		return nil
	}
	cached.once.Do(func() {
		// Parse the complete rule once, including definitions outside the entry.
		// Invalid descriptions must not become valid by bypassing construction.
		root, err := base.ParseRule(rulePath)
		if err != nil {
			return
		}
		if document, ok := root.Origin.(yaml.MapSlice); ok {
			cached.decoders = compileStructuredEntries(document)
		}
	})
	return cached.decoders[entry]
}

// Enclosing operators, references, length overrides, custom parsers, duplicate
// assignments and child fields could change a bridge's meaning. Only the exact
// simple enclosing grammar is eligible. Carrier programs retain their trials,
// recovery and raw fallback through the ordinary parser.
func compileStructuredEntries(document yaml.MapSlice) map[string]stream_parser.StructuredDecoder {
	var entries yaml.MapSlice
	var endian, unit bool
	seen := make(map[string]bool, len(document))
	for _, item := range document {
		key, ok := item.Key.(string)
		if !ok || seen[key] {
			return nil
		}
		seen[key] = true
		switch key {
		case "endian":
			if item.Value != "big" && item.Value != "little" {
				return nil
			}
			endian = true
		case "unit":
			if item.Value != "byte" {
				return nil
			}
			unit = true
		case "Package":
			// Inherited settings are applied in document order. The native
			// helper owns field endianness, but enclosing bounds must be known.
			if !endian || !unit {
				return nil
			}
			entries, ok = item.Value.(yaml.MapSlice)
			if !ok {
				return nil
			}
		default:
			if !structuredDefinitionName(key) {
				return nil
			}
		}
	}
	if !endian || !unit || entries == nil {
		return nil
	}
	decoders := make(map[string]stream_parser.StructuredDecoder)
	clear(seen)
	for _, entry := range entries {
		name, ok := entry.Key.(string)
		if !ok || seen[name] || !structuredDefinitionName(name) {
			return nil
		}
		seen[name] = true
		body, ok := entry.Value.(yaml.MapSlice)
		if !ok || len(body) != 1 || body[0].Key != "operator" {
			continue
		}
		source, ok := body[0].Value.(string)
		if !ok {
			continue
		}
		if decoder := stream_parser.StructuredDecoderForProgram(source); decoder != nil {
			decoders[name] = decoder
		}
	}
	return decoders
}

func structuredDefinitionName(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}
