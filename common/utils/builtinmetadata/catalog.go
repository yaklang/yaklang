// Package builtinmetadata holds build-time analysis results for embedded Yak
// scripts. Runtime callers must check the source digest before using an entry.
package builtinmetadata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
)

//go:generate bash ../../../scripts/generate-builtin-metadata.sh

const Version = 2

// Metadata contains only derived, immutable script information, never database
// IDs, favorites, authorship, or other user-owned state.
type Metadata struct {
	VerboseName       string `json:"verbose_name,omitempty"`
	VerboseNameZh     string `json:"verbose_name_zh,omitempty"`
	Description       string `json:"description,omitempty"`
	Keywords          string `json:"keywords,omitempty"`
	Usage             string `json:"usage,omitempty"`
	Parameters        string `json:"parameters"`
	EnvironmentKeys   string `json:"environment_keys,omitempty"`
	EnableAIOutputLog int    `json:"enable_ai_output_log,omitempty"`
}

type Entry struct {
	SourceSHA256 string   `json:"source_sha256"`
	Metadata     Metadata `json:"metadata"`
}

type Catalog struct {
	Version        int               `json:"version"`
	Entries        map[string]Entry  `json:"entries"`
	SourceHashes   map[string]string `json:"source_hashes,omitempty"`
	ResourceHashes map[string]string `json:"resource_hashes,omitempty"`
}

func SourceHash(source string) string {
	hash := sha256.Sum256([]byte(source))
	return hex.EncodeToString(hash[:])
}

func (c *Catalog) Lookup(key, source string) (Metadata, bool) {
	if c == nil || c.Version != Version {
		return Metadata{}, false
	}
	entry, ok := c.Entries[key]
	if !ok || entry.SourceSHA256 != SourceHash(source) || !validMetadata(key, entry.Metadata) {
		return Metadata{}, false
	}
	return entry.Metadata, true
}

var loadOnce sync.Once
var catalog Catalog

func loadCatalog() *Catalog {
	loadOnce.Do(func() {
		if json.Unmarshal(embeddedCatalog, &catalog) != nil {
			catalog = Catalog{}
		}
	})
	return &catalog
}

// Lookup returns a value copy. Missing, stale, or malformed generated data is a
// cache miss: callers retain their normal dynamic analysis path.
func Lookup(key, source string) (Metadata, bool) {
	if !Enabled {
		return Metadata{}, false
	}
	return loadCatalog().Lookup(key, source)
}

// ResourceHash also versions the derived metadata. An extractor change can
// alter a schema without changing its .yak source, and must trigger one resync.
// Unrelated resource kinds do not invalidate each other's synchronization.
func ResourceHash(sourceHash, prefix string) string {
	if Enabled {
		c := loadCatalog()
		if c.Version == Version && c.SourceHashes[prefix] == sourceHash && c.ResourceHashes[prefix] != "" {
			return c.ResourceHashes[prefix]
		}
	}
	return sourceHash
}

func (c *Catalog) revision(prefix string) string {
	entries := map[string]Entry{}
	for key, entry := range c.Entries {
		if strings.HasPrefix(key, prefix) {
			entries[key] = entry
		}
	}
	data, _ := json.Marshal(Catalog{Version: c.Version, Entries: entries})
	return SourceHash(string(data))
}

// PrepareResourceHashes runs in the generator, never on the startup path.
func (c *Catalog) PrepareResourceHashes(sources map[string]string) {
	c.SourceHashes = sources
	c.ResourceHashes = make(map[string]string, len(sources))
	for prefix, source := range sources {
		c.ResourceHashes[prefix] = SourceHash(source + "\x00" + c.revision(prefix))
	}
}

func validMetadata(key string, m Metadata) bool {
	if strings.HasPrefix(key, "tool:") {
		var schema map[string]json.RawMessage
		return (m.EnableAIOutputLog == 1 || m.EnableAIOutputLog == 2) && json.Unmarshal([]byte(m.Parameters), &schema) == nil && schema != nil
	}
	if !strings.HasPrefix(key, "plugin:") && !strings.HasPrefix(key, "forge:") {
		return false
	}
	if strings.HasPrefix(key, "plugin:") && m.EnvironmentKeys != "" {
		var encoded string
		var keys []string
		if json.Unmarshal([]byte(m.EnvironmentKeys), &encoded) != nil || json.Unmarshal([]byte(encoded), &keys) != nil {
			return false
		}
	}
	params := m.Parameters
	if params == "" {
		return true
	} // An empty parameter list is emitted as an empty string.
	if strings.HasPrefix(key, "plugin:") {
		var unquoted string
		if json.Unmarshal([]byte(params), &unquoted) != nil {
			return false
		}
		params = unquoted
	}
	var fields []json.RawMessage
	return json.Unmarshal([]byte(params), &fields) == nil
}
