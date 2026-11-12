// Package loader holds the global catalog and the input-opening pipeline.
// Loaders are aligned with file formats and parsers with instruction-set
// architectures; the two registries are independent of each other.
package loader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch"
	"github.com/yaklang/yaklang/common/yak/reify/loader/fileformat"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported input format")
	ErrAmbiguousFormat   = errors.New("multiple loaders match the input")
)

// Catalog is an immutable set of loaders and parsers. Each Format maps to one
// loader and each Architecture maps to at most one parser.
type Catalog struct {
	loaders map[core.Format]core.Loader
	parsers map[core.Architecture]core.Parser
}

// Default is the process-wide catalog: one loader per native format and one
// parser per supported architecture. Registration is explicit; no package init
// hook mutates it later.
var Default = mustNewCatalog(fileformat.All(), arch.All())

func mustNewCatalog(loaders []core.Loader, parsers []core.Parser) *Catalog {
	catalog, err := NewCatalog(loaders, parsers)
	if err != nil {
		panic("reify: invalid default catalog: " + err.Error())
	}
	return catalog
}

func NewCatalog(loaders []core.Loader, parsers []core.Parser) (*Catalog, error) {
	catalog := &Catalog{
		loaders: make(map[core.Format]core.Loader, len(loaders)),
		parsers: make(map[core.Architecture]core.Parser, len(parsers)),
	}
	for _, loader := range loaders {
		if loader == nil {
			return nil, fmt.Errorf("cannot register a nil loader")
		}
		kind := loader.Kind()
		if kind == "" || kind == core.FormatUnknown {
			return nil, fmt.Errorf("loader requires a format kind")
		}
		if _, exists := catalog.loaders[kind]; exists {
			return nil, fmt.Errorf("duplicate loader for format %s", kind)
		}
		catalog.loaders[kind] = loader
	}
	for _, parser := range parsers {
		if parser == nil {
			return nil, fmt.Errorf("cannot register a nil parser")
		}
		kind := parser.Kind()
		if kind == "" || kind == core.ArchUnknown {
			return nil, fmt.Errorf("parser requires an architecture kind")
		}
		if _, exists := catalog.parsers[kind]; exists {
			return nil, fmt.Errorf("duplicate parser for architecture %s", kind)
		}
		catalog.parsers[kind] = parser
	}
	return catalog, nil
}

func (c *Catalog) Loaders() []core.LoaderInfo {
	result := make([]core.LoaderInfo, 0, len(c.loaders))
	for kind, loader := range c.loaders {
		result = append(result, core.LoaderInfo{Kind: kind, Name: loader.Name(), Description: loader.Description()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Kind < result[j].Kind })
	return result
}

func (c *Catalog) Parsers() []core.ParserInfo {
	result := make([]core.ParserInfo, 0, len(c.parsers))
	for kind, parser := range c.parsers {
		result = append(result, core.ParserInfo{Kind: kind, Name: parser.Name(), Available: parser.Available()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Kind < result[j].Kind })
	return result
}

func (c *Catalog) Loader(kind core.Format) (core.Loader, bool) {
	loader, ok := c.loaders[kind]
	return loader, ok
}

func (c *Catalog) Parser(kind core.Architecture) (core.Parser, bool) {
	parser, ok := c.parsers[kind]
	return parser, ok
}

// CheckResult combines loader-level header facts with the parser selected for
// the reported architecture.
type CheckResult struct {
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	Loader      core.LoaderInfo   `json:"loader"`
	Kind        core.InputKind    `json:"kind"`
	Format      core.Format       `json:"format"`
	Target      core.Target       `json:"target"`
	ObjectType  string            `json:"object_type,omitempty"`
	Sections    int               `json:"sections"`
	Parser      *core.ParserInfo  `json:"parser,omitempty"`
	Diagnostics []core.Diagnostic `json:"diagnostics,omitempty"`
}

// CheckFile validates a regular file's header and reports the selected loader
// and parser. It does not decode instructions.
func (c *Catalog) CheckFile(ctx context.Context, path string) (*CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, size, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return c.Check(ctx, path, file, size)
}

// Check validates a borrowed source. The source is not closed.
func (c *Catalog) Check(ctx context.Context, name string, src io.ReaderAt, size int64) (*CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := core.NewByteReader(name, src, size)
	if err != nil {
		return nil, err
	}
	loader, err := c.detect(reader)
	if err != nil {
		return nil, err
	}
	checked, err := loader.Check(reader)
	if err != nil {
		return nil, fmt.Errorf("loader %s: %w", loader.Name(), err)
	}
	if checked == nil {
		return nil, fmt.Errorf("loader %s returned no check result", loader.Name())
	}
	parserInfo, _, diagnostics := c.selectParser(checked.Target)
	return &CheckResult{
		Path:        name,
		Size:        size,
		Loader:      core.LoaderInfo{Kind: loader.Kind(), Name: loader.Name(), Description: loader.Description()},
		Kind:        checked.Kind,
		Format:      checked.Format,
		Target:      checked.Target,
		ObjectType:  checked.ObjectType,
		Sections:    checked.Sections,
		Parser:      parserInfo,
		Diagnostics: append(append([]core.Diagnostic(nil), checked.Diagnostics...), diagnostics...),
	}, nil
}

// OpenFile loads a regular file and returns an artifact that owns the open
// source. Callers must close the artifact.
func (c *Catalog) OpenFile(ctx context.Context, path string) (*core.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, size, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	artifact, err := c.Open(ctx, path, file, size)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if artifact.Reader != nil {
		artifact.Reader = artifact.Reader.WithOwner()
	}
	return artifact, nil
}

// Open loads a borrowed source. The caller retains ownership of src.
func (c *Catalog) Open(ctx context.Context, name string, src io.ReaderAt, size int64) (*core.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := core.NewByteReader(name, src, size)
	if err != nil {
		return nil, err
	}
	loader, err := c.detect(reader)
	if err != nil {
		return nil, err
	}
	artifact, err := loader.Load(ctx, reader)
	if err != nil {
		return nil, fmt.Errorf("loader %s: %w", loader.Name(), err)
	}
	if artifact == nil {
		return nil, fmt.Errorf("loader %s returned no artifact", loader.Name())
	}
	artifact.Origin = core.Origin{Name: name, Size: size}
	artifact.Format = loader.Kind()
	artifact.Loader = loader
	if artifact.Reader == nil {
		artifact.Reader = reader
	}
	_, parser, diagnostics := c.selectParser(artifact.Target)
	artifact.Parser = parser
	artifact.Diagnostics = append(artifact.Diagnostics, diagnostics...)
	if err := core.ValidateArtifact(artifact); err != nil {
		return nil, fmt.Errorf("loader %s metadata: %w", loader.Name(), err)
	}
	return artifact, nil
}

func (c *Catalog) detect(reader *core.ByteReader) (core.Loader, error) {
	probeSize := core.ProbeSize
	if int64(probeSize) > reader.Size() {
		probeSize = int(reader.Size())
	}
	prefix, err := reader.Bytes(0, uint64(probeSize))
	if err != nil {
		return nil, err
	}
	var matches []core.Loader
	for _, loader := range c.loaders {
		if loader.Match(prefix) {
			matches = append(matches, loader)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, reader.Name())
	case 1:
		return matches[0], nil
	default:
		kinds := make([]string, 0, len(matches))
		for _, loader := range matches {
			kinds = append(kinds, string(loader.Kind()))
		}
		sort.Strings(kinds)
		return nil, fmt.Errorf("%w: %s", ErrAmbiguousFormat, strings.Join(kinds, ", "))
	}
}

// selectParser returns parser metadata even when the parser cannot decode the
// target. Diagnostics explain a missing, incompatible or unavailable decoder.
func (c *Catalog) selectParser(target core.Target) (*core.ParserInfo, core.Parser, []core.Diagnostic) {
	parser, ok := c.parsers[target.Architecture]
	if !ok {
		return nil, nil, []core.Diagnostic{{
			Code:    "parser-unavailable",
			Message: fmt.Sprintf("No parser is registered for architecture %s", target.Architecture),
		}}
	}
	info := &core.ParserInfo{Kind: parser.Kind(), Name: parser.Name(), Available: parser.Available()}
	if err := parser.Check(target); err != nil {
		return info, nil, []core.Diagnostic{{
			Code:    "parser-incompatible",
			Message: fmt.Sprintf("Parser %s does not accept target %s/%d-bit: %v", parser.Name(), target.Architecture, target.Bits, err),
		}}
	}
	if !parser.Available() {
		return info, parser, []core.Diagnostic{{
			Code:    "decoder-unavailable",
			Message: fmt.Sprintf("Parser %s is registered but cannot decode instructions yet", parser.Name()),
		}}
	}
	return info, parser, nil
}

func openRegular(path string) (*os.File, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("input is not a regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err = file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, 0, fmt.Errorf("input is not a regular file: %s", path)
	}
	return file, info.Size(), nil
}
