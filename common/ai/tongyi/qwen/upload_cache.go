package qwen

import (
	// #nosec G505
	"crypto/sha1"
	"encoding/hex"
	"time"
)

// UploadCacher is an interface for caching uploaded file url to prevent duplicate upload.
// By default we provide Sample MemoryFileCache as the implementation.
// Customize your own cache manager by implementing this interface.
type UploadCacher interface {
	SaveCache(buf []byte, url string) error
	GetCache(buf []byte) string
}

// ==================== Sample MemoryFileCache ====================.
type FileCacheInfo struct {
	URL        string
	UploadTime int64
}

// MemoryFileCache is a simple in-memory-cache implementation for UploadCacher interface.
type MemoryFileCache struct {
	MapFiles             map[string]*FileCacheInfo
	MaxFileCacheLifeTime time.Duration
}

func NewMemoryFileCache() *MemoryFileCache {
	mgr := &MemoryFileCache{
		MapFiles:             make(map[string]*FileCacheInfo),
		MaxFileCacheLifeTime: time.Hour*2 - time.Minute*5,
	}

	// cron job to clean up outdated memory cache.
	go mgr.cronMemoryCleaner()

	return mgr
}

func (mgr *MemoryFileCache) SaveCache(buf []byte, url string) error {
	key := mgr.hash(buf)

	mgr.MapFiles[key] = &FileCacheInfo{
		URL:        url,
		UploadTime: time.Now().Unix(),
	}

	return nil
}

func (mgr *MemoryFileCache) GetCache(buf []byte) string {
	key := mgr.hash(buf)

	cache, isok := mgr.MapFiles[key]
	if isok {
		return cache.URL
	}

	return ""
}

func (mgr *MemoryFileCache) cronMemoryCleaner() {
	for {
		time.Sleep(time.Minute * 5)

		curtime := time.Now().Unix()

		for k, v := range mgr.MapFiles {
			if curtime-v.UploadTime > int64(mgr.MaxFileCacheLifeTime) {
				delete(mgr.MapFiles, k)
			}
		}
	}
}

func (mgr *MemoryFileCache) hash(buf []byte) string {
	// #nosec G401
	h := sha1.New()
	h.Write(buf)
	return hex.EncodeToString(h.Sum(nil))
}
