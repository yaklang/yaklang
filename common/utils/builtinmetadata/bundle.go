// Package builtinmetadata verifies compressed analysis results against the
// exact script set that produced them. Its hash is the SHA-256 of the compressed file.
package builtinmetadata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:generate bash ../../../scripts/generate-builtin-metadata.sh

// Bump Version when extraction semantics change, even if scripts do not.
const Version = 3

type Metadata struct {
	VerboseName       string          `json:"verbose_name,omitempty"`
	VerboseNameZh     string          `json:"verbose_name_zh,omitempty"`
	Description       string          `json:"description,omitempty"`
	Keywords          string          `json:"keywords,omitempty"`
	Usage             string          `json:"usage,omitempty"`
	Parameters        json.RawMessage `json:"parameters,omitempty"`
	EnvironmentKeys   json.RawMessage `json:"environment_keys,omitempty"`
	EnableAIOutputLog int             `json:"enable_ai_output_log,omitempty"`
	SyntaxFlowRule    json.RawMessage `json:"syntaxflow_rule,omitempty"`
}

type File struct {
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type Entry struct {
	Script        string          `json:"script"`
	Metadata      json.RawMessage `json:"metadata"`
	SourceStrings []SourceString  `json:"source_strings,omitempty"`
}

type Bundle struct {
	Version    int              `json:"version"`
	Kind       string           `json:"kind"`
	Recipe     string           `json:"recipe"`
	SourceHash string           `json:"source_hash"`
	Files      map[string]File  `json:"files"`
	Entries    map[string]Entry `json:"entries"`
}

func Digest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func SourceHash(source string) string { return Digest([]byte(source)) }

func Fingerprint(files map[string]File, recipe string) string {
	data, _ := json.Marshal(struct {
		Version int
		Recipe  string
		Files   map[string]File
	}{Version, recipe, files})
	return Digest(data)
}

func (b *Bundle) Matches(files map[string]File, recipe string) bool {
	return b != nil && b.Version == Version && b.Recipe == recipe && b.SourceHash == Fingerprint(files, recipe)
}

// lookup uses the manifest and metadata already validated by Decode. Only
// the caller's source needs checking on each lookup.
func (b *Bundle) lookup(key, source string) (Metadata, bool) {
	if b == nil || b.Version != Version {
		return Metadata{}, false
	}
	e, ok := b.Entries[key]
	if !ok || b.Files[e.Script].SHA256 != SourceHash(source) {
		return Metadata{}, false
	}

	metadata, err := e.resolve(source)
	return metadata, err == nil && validMetadata(key, metadata)
}

// Text keeps one file/entry per line: readable JSON with local, stable diffs,
// without indenting thousands of instructions or escaping nested JSON strings.
func Text(b *Bundle) ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("{\n")
	for i, key := range []string{"version", "kind", "recipe", "source_hash", "files", "entries"} {
		if i > 0 {
			out.WriteString(",\n")
		}
		fmt.Fprintf(&out, "  %q: ", key)
		if key != "files" && key != "entries" {
			out.Write(fields[key])
			continue
		}
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(fields[key], &entries); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(entries))
		for k := range entries {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out.WriteString("{\n")
		for j, k := range keys {
			if j > 0 {
				out.WriteString(",\n")
			}
			name, _ := json.Marshal(k)
			out.WriteString("    ")
			out.Write(name)
			out.WriteString(": ")
			out.Write(entries[k])
		}
		out.WriteString("\n  }")
	}
	out.WriteString("\n}\n")
	return out.Bytes(), nil
}

// Parse validates the checked-in text without invoking any script parser.
func Parse(raw []byte, kind string) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	if b.Kind != kind || b.Version != Version || len(b.Files) == 0 || b.SourceHash != Fingerprint(b.Files, b.Recipe) {
		return nil, fmt.Errorf("invalid %s metadata manifest", kind)
	}
	for name, file := range b.Files {
		if !fs.ValidPath(name) || (!fs.FileMode(file.Mode).IsDir() && len(file.SHA256) != 64) {
			return nil, fmt.Errorf("invalid script fingerprint: %s", name)
		}
	}
	if err := b.validateEntries(); err != nil {
		return nil, err
	}
	return &b, nil
}

func (b *Bundle) validateEntries() error {
	for key, e := range b.Entries {
		file, ok := b.Files[e.Script]
		var metadata Metadata
		if !ok || fs.FileMode(file.Mode).IsDir() || kindForKey(key) != b.Kind || json.Unmarshal(e.Metadata, &metadata) != nil || !validMetadata(key, metadata) {
			return fmt.Errorf("invalid script/metadata mapping: %s", key)
		}
		if err := e.validate(file.Size); err != nil {
			return err
		}
	}
	return nil
}

func kindForKey(key string) string {
	prefix, _, _ := strings.Cut(key, ":")
	switch prefix {
	case "tool":
		return "aitool"
	case "plugin":
		return "coreplugin"
	case "forge":
		return "forge"
	case "sf":
		return "syntaxflow"
	}
	return ""
}

func validMetadata(key string, m Metadata) bool {
	switch kindForKey(key) {
	case "syntaxflow":
		return len(m.SyntaxFlowRule) > 0 && json.Valid(m.SyntaxFlowRule)
	case "aitool":
		var object map[string]json.RawMessage
		return (m.EnableAIOutputLog == 1 || m.EnableAIOutputLog == 2) && json.Unmarshal(m.Parameters, &object) == nil && object != nil
	case "coreplugin", "forge":
		if len(m.EnvironmentKeys) > 0 {
			var keys []string
			if json.Unmarshal(m.EnvironmentKeys, &keys) != nil || keys == nil {
				return false
			}
		}
		if len(m.Parameters) == 0 {
			return true
		}
		var fields []json.RawMessage
		return json.Unmarshal(m.Parameters, &fields) == nil && fields != nil
	}
	return false
}
