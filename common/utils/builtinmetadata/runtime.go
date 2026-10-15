package builtinmetadata

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"sync"
)

type resource struct {
	once    sync.Once
	archive func() ([]byte, error)
	prefix  string
	bundle  *Bundle
	hash    string
	err     error
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
		b, err := Decode(data, kind, expected)
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
	return r.hash, r.err
}

func Lookup(key, source string) (Metadata, bool) {
	if !Enabled {
		return Metadata{}, false
	}
	return load(kindForKey(key)).bundle.lookup(key, source)
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
