package minirehs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LoadOrCompile loads an image whose ordered patterns and effective MVS options
// match the request, or compiles and atomically replaces a missing, obsolete or
// corrupt cache. Non-MVS backends compile without accessing the cache. Cache I/O
// failures are returned to the caller; LoadImage itself never falls back to Compile.
//
// Writers coordinate with an exclusively created path+".lock" file. A lock left
// by a terminated process causes a bounded wait (two minutes), then an error;
// remove that lock only after establishing that its writer is no longer running.
func LoadOrCompile(path string, patterns []Pattern, opts ...Option) (Database, error) {
	if path == "" {
		return nil, fmt.Errorf("minirehs: empty image cache path")
	}
	cfg := newDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	b, err := selectBackend(cfg)
	if err != nil {
		return nil, err
	}
	if b.kind() != BackendMVS {
		return compileWithConfig(patterns, cfg)
	}
	flags := compileImageFlags(cfg)
	if flags & ^uint32(imageReportLocation|imageBiAnchor) != 0 {
		return nil, ErrIncompatibleImage
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	hash := imageIdentity(patterns, cfg, flags)
	read := func() (Database, error) {
		data, e := readImageFile(path)
		if errors.Is(e, os.ErrNotExist) {
			return nil, nil
		}
		if errors.Is(e, ErrCorruptImage) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		h, e := readImageHeader(data)
		if e != nil || h.hash != hash || h.flags != flags || h.minLiteralLen != cfg.minLiteralLen || h.n != len(patterns) {
			return nil, nil
		}
		db, e := loadCompilationImage(data, h)
		if errors.Is(e, ErrCorruptImage) || errors.Is(e, ErrIncompatibleImage) {
			return nil, nil
		}
		return db, e
	}
	if db, e := read(); db != nil || e != nil {
		return db, e
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	unlock, err := lockImageCache(path+".lock", 2*time.Minute)
	if err != nil {
		return nil, err
	}
	defer unlock()
	// Another process may have populated the cache while we waited.
	if db, e := read(); db != nil || e != nil {
		return db, e
	}
	db, err := compileWithConfig(patterns, cfg)
	if err != nil {
		return nil, err
	}
	data, err := db.MarshalImage()
	if err == nil {
		err = writeImageCache(path, data)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func lockImageCache(path string, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	var transientSince time.Time
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			if err = f.Close(); err != nil {
				_ = os.Remove(path)
				return nil, err
			}
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			// CREATE_NEW can report ACCESS_DENIED while a Windows lock name
			// is pending deletion. Retry that brief state; a real ACL denial
			// still returns an I/O error after at most one second.
			if !transientImageLockError(err) {
				return nil, err
			}
			if transientSince.IsZero() {
				transientSince = time.Now()
			}
			if time.Since(transientSince) >= time.Second {
				return nil, err
			}
		} else {
			transientSince = time.Time{}
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("minirehs: timed out waiting for image cache lock %q", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func writeImageCache(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0644); err != nil {
		return fmt.Errorf("minirehs: chmod image temporary file: %w", err)
	}
	if _, err = f.Write(data); err != nil {
		return fmt.Errorf("minirehs: write image temporary file: %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("minirehs: sync image temporary file: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("minirehs: close image temporary file: %w", err)
	}
	if err = replaceImageFile(f.Name(), path); err != nil {
		time.Sleep(10 * time.Millisecond)
		err = replaceImageFile(f.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("minirehs: replace image cache: %w", err)
	}
	return nil
}
