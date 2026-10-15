package builtinmetadata

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

type lazyEntry struct {
	once     sync.Once
	entry    Entry
	metadata Metadata
	err      error
}

type resource struct {
	once          sync.Once
	entriesOnce   sync.Once
	mu            sync.RWMutex
	archive       func() ([]byte, error)
	prefix        string
	bundle        *Bundle
	hash          string
	err           error
	entriesErr    error
	packedEntries []byte
	entries       map[string]*lazyEntry
}

var resources sync.Map

// Register connects the metadata to the actual gzip script archive in this
// binary. Validation is lazy and shared by sync-hash and metadata lookups.
func Register(kind, prefix string, archive func() ([]byte, error)) {
	resources.LoadOrStore(kind, &resource{archive: archive, prefix: prefix})
}

func load(kind string) *resource {
	value, ok := resources.Load(kind)
	if !ok {
		return &resource{err: fmt.Errorf("unregistered script set: %s", kind)}
	}
	r := value.(*resource)
	r.once.Do(func() {
		defer func() {
			if r.err != nil {
				log.Warnf("builtin %s metadata validation failed, using dynamic analysis: %v", kind, r.err)
			}
		}()
		data, expected, err := embeddedMetadata(kind)
		if err != nil {
			r.err = err
			return
		}
		b, entries, err := decodeIndex(data, kind, expected)
		if err != nil {
			r.err = err
			return
		}
		archive, err := r.archive()
		if err == nil {
			err = b.ValidateArchive(archive, r.prefix)
		}
		if err != nil {
			r.err = err
			return
		}
		r.bundle = b
		r.packedEntries = entries
		r.hash = expected
	})
	return r
}

// Hash is exactly the compressed metadata hash, after checking that the full
// script archive corresponds to its manifest. Invalid metadata has no sync hash.
func Hash(kind string) (string, error) {
	if !Enabled {
		return "", fmt.Errorf("metadata is disabled in ordinary builds")
	}
	r := load(kind)
	if r.err != nil {
		return "", r.err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.entriesErr != nil {
		return "", r.entriesErr
	}
	return r.hash, nil
}

// Invalidate withdraws the sync hash if a domain-specific decoder rejects a
// lazily loaded value. Successful dynamic fallback must not bless that cache.
func Invalidate(kind string, err error) {
	if err == nil {
		return
	}
	if value, ok := resources.Load(kind); ok {
		r := value.(*resource)
		r.mu.Lock()
		first := r.entriesErr == nil
		r.entriesErr = err
		r.mu.Unlock()
		if first {
			log.Warnf("builtin %s metadata invalid, using dynamic analysis: %v", kind, err)
		}
	}
}

func Lookup(key, source string) (Metadata, bool) {
	if !Enabled {
		return Metadata{}, false
	}
	r := load(kindForKey(key))
	if r.err != nil {
		return Metadata{}, false
	}
	r.entriesOnce.Do(func() {
		err := r.bundle.decodeEntries(r.packedEntries)
		r.packedEntries = nil
		if err == nil {
			r.entries = make(map[string]*lazyEntry, len(r.bundle.Entries))
			for key, entry := range r.bundle.Entries {
				r.entries[key] = &lazyEntry{entry: entry}
			}
		}
		r.bundle.Entries = nil
		if err != nil {
			Invalidate(kindForKey(key), err)
		}
	})
	r.mu.RLock()
	err := r.entriesErr
	r.mu.RUnlock()
	if err != nil {
		return Metadata{}, false
	}
	entry, ok := r.entries[key]
	if !ok || r.bundle.Files[entry.entry.Script].SHA256 != SourceHash(source) {
		return Metadata{}, false
	}
	entry.once.Do(func() {
		entry.metadata, entry.err = entry.entry.resolve(source)
		if entry.err == nil && !validMetadata(key, entry.metadata) {
			entry.err = fmt.Errorf("invalid metadata entry: %s", key)
		}
		// Retain either the unresolved template or the resolved value, never both.
		entry.entry.Metadata, entry.entry.SourceStrings = nil, nil
		if entry.err != nil {
			Invalidate(kindForKey(key), entry.err)
		}
	})
	return entry.metadata, entry.err == nil
}

// SyncHash retains ordinary-build behavior. An invalid metadata bundle
// returns no marker: callers use dynamic analysis and must not mark it current.
func SyncHash(kind, ordinaryHash string) string {
	if !Enabled {
		return ordinaryHash
	}
	hash, _ := Hash(kind)
	return hash
}
