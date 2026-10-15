// Package builtinmetadata verifies compressed analysis results against the
// exact script set that produced them. Its hash is the SHA-256 of the gzip file.
package builtinmetadata

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

//go:generate bash ../../../scripts/generate-builtin-metadata.sh

// Bump Version when extraction semantics change, even if scripts do not.
const Version = 1

type Metadata struct {
	VerboseName       string `json:"verbose_name,omitempty"`
	VerboseNameZh     string `json:"verbose_name_zh,omitempty"`
	Description       string `json:"description,omitempty"`
	Keywords          string `json:"keywords,omitempty"`
	Usage             string `json:"usage,omitempty"`
	Parameters        string `json:"parameters,omitempty"`
	EnvironmentKeys   string `json:"environment_keys,omitempty"`
	EnableAIOutputLog int    `json:"enable_ai_output_log,omitempty"`
	SyntaxFlowRule    string `json:"syntaxflow_rule,omitempty"`
}

type File struct {
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type Entry struct {
	Script       string   `json:"script"`
	SourceSHA256 string   `json:"source_sha256"`
	Metadata     Metadata `json:"metadata"`
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
	if !ok || e.SourceSHA256 != SourceHash(source) {
		return Metadata{}, false
	}

	return e.Metadata, true
}

func Encode(b *Bundle) ([]byte, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Decode(data []byte, kind, expectedHash string) (*Bundle, error) {
	if expectedHash != "" && Digest(data) != expectedHash {
		return nil, fmt.Errorf("%s metadata hash mismatch", kind)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, 128<<20))
	if err != nil {
		return nil, err
	}
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
	for key, e := range b.Entries {
		file, ok := b.Files[e.Script]
		if !ok || file.SHA256 != e.SourceSHA256 || kindForKey(key) != kind || !validMetadata(key, e.Metadata) {
			return nil, fmt.Errorf("invalid script/metadata mapping: %s", key)
		}
	}
	return &b, nil
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
		return m.SyntaxFlowRule != "" && json.Valid([]byte(m.SyntaxFlowRule))
	case "aitool":
		var object map[string]json.RawMessage
		return (m.EnableAIOutputLog == 1 || m.EnableAIOutputLog == 2) && json.Unmarshal([]byte(m.Parameters), &object) == nil && object != nil
	case "coreplugin", "forge":
		if m.EnvironmentKeys != "" {
			var encoded string
			var keys []string
			if json.Unmarshal([]byte(m.EnvironmentKeys), &encoded) != nil || json.Unmarshal([]byte(encoded), &keys) != nil {
				return false
			}
		}
		if m.Parameters == "" {
			return true
		}
		params := m.Parameters
		if kindForKey(key) == "coreplugin" {
			var encoded string
			if json.Unmarshal([]byte(params), &encoded) != nil {
				return false
			}
			params = encoded
		}
		var fields []json.RawMessage
		return json.Unmarshal([]byte(params), &fields) == nil
	}
	return false
}
