package aibalanceclient

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const ClientUserIDHeader = "X-User-Id"

const clientUserIDFilePrefix = "aibalance-client-trace-id-"

var clientUserIDCache struct {
	sync.Mutex
	db   *sql.DB
	file string
	id   string
}

// GetOrCreateClientUserID returns this Profile's persistent UUID. The Profile
// KV is authoritative; a file in yakit-projects/temp (or YAKIT_HOME/temp) keeps
// the same identity available when the database cannot be read or written.
// It returns empty only when neither storage can provide a durable identity.
func GetOrCreateClientUserID() string {
	clientUserIDCache.Lock()
	defer clientUserIDCache.Unlock()

	db := consts.GetGormProfileDatabase()
	file := clientUserIDFile(consts.GetCurrentProfileDatabasePath(), consts.GetDefaultYakitBaseTempDir())
	var sqlDB *sql.DB
	if db != nil {
		sqlDB = db.DB()
	}
	if clientUserIDCache.id != "" && clientUserIDCache.db == sqlDB && clientUserIDCache.file == file {
		return clientUserIDCache.id
	}

	id, inDB, err := loadClientUserID(db, file)
	if err != nil {
		log.Debugf("AIBalance client UUID persistence: %v", err)
	}
	// Retry file-only results against the DB on later calls, so recovery can
	// seed a missing KV or pick up the existing authoritative value.
	if id != "" && inDB {
		clientUserIDCache.db = sqlDB
		clientUserIDCache.file = file
		clientUserIDCache.id = id
	}
	return id
}

func clientUserIDFile(profilePath, tempDir string) string {
	abs, err := filepath.Abs(profilePath)
	if err == nil {
		profilePath = abs
	}
	// Independent Profiles in the same YAKIT_HOME must not share the fallback.
	hash := sha256.Sum256([]byte(filepath.Clean(profilePath)))
	return filepath.Join(tempDir, fmt.Sprintf("%s%x", clientUserIDFilePrefix, hash[:]))
}

// ClearTempDirPreservingClientUserIDs clears version-dependent caches while
// retaining every Profile's fallback identity. Explicit user data resets may
// still remove the entire directory.
func ClearTempDirPreservingClientUserIDs(tempDir string) error {
	entries, err := os.ReadDir(tempDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		hash, hasPrefix := strings.CutPrefix(entry.Name(), clientUserIDFilePrefix)
		if hasPrefix && len(hash) == sha256.Size*2 && entry.Type().IsRegular() {
			if _, err := hex.DecodeString(hash); err == nil {
				continue
			}
		}
		if err := os.RemoveAll(filepath.Join(tempDir, entry.Name())); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func readClientUserID(db *gorm.DB) (string, error) {
	kv, err := yakit.GetKeyModel(db, TRACE_ID_DB_KEY)
	if gorm.IsRecordNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	id, err := strconv.Unquote(kv.Value)
	if err != nil {
		// Match GetKey's handling of legacy unquoted values.
		id = kv.Value
	}
	if id == "" {
		return "", fmt.Errorf("stored client UUID is empty")
	}
	return id, nil
}

// loadClientUserID returns only IDs read from durable storage. The bool marks
// successful DB persistence, allowing the caller to cache only that case.
func loadClientUserID(db *gorm.DB, file string) (string, bool, error) {
	stored, dbErr := readClientUserID(db)
	if stored != "" {
		// Mirror existing Profiles before a future cold start with a broken DB.
		return stored, true, writeClientUserIDFile(file, stored, true)
	}

	candidate, fileErr := readClientUserIDFile(file)
	if os.IsNotExist(fileErr) {
		generated, err := uuid.NewRandom()
		if err != nil {
			return "", false, err
		}
		candidate = generated.String()
		fileErr = writeClientUserIDFile(file, candidate, false)
		if fileErr == nil {
			// Atomic no-replace publication may have lost to another process.
			candidate, fileErr = readClientUserIDFile(file)
		}
	}
	if dbErr != nil {
		if fileErr != nil {
			return "", false, fmt.Errorf("database: %v; fallback file: %w", dbErr, fileErr)
		}
		return candidate, false, dbErr
	}

	if candidate == "" {
		// A broken fallback must not prevent a healthy database from creating
		// its identity. Return this candidate only after a successful DB reread.
		generated, err := uuid.NewRandom()
		if err != nil {
			return "", false, err
		}
		candidate = generated.String()
	}
	if err := yakit.SetKeyIfAbsent(db, TRACE_ID_DB_KEY, candidate); err != nil {
		if fileErr == nil {
			return candidate, false, err
		}
		return "", false, err
	}
	stored, err := readClientUserID(db)
	if err != nil || stored == "" {
		if fileErr == nil {
			return candidate, false, err
		}
		return "", false, err
	}
	return stored, true, writeClientUserIDFile(file, stored, true)
}

func readClientUserIDFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("invalid client UUID in fallback file: %w", err)
	}
	return id, nil
}

// Publish a complete, synced file. O_EXCL on the destination alone would let
// another process observe an empty/partial ID, or leave one after a crash.
// Linking the temporary file is atomic and never overwrites a concurrent winner.
func writeClientUserIDFile(file, id string, replace bool) error {
	if existing, err := readClientUserIDFile(file); err == nil && (existing == id || !replace) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".aibalance-trace-id-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(id + "\n"); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if replace {
		return os.Rename(tmp.Name(), file)
	}
	if err := os.Link(tmp.Name(), file); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}
