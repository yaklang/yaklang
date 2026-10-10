package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
)

// InstanceManager borrows the profile DB, owns its index handles, and performs
// lightweight startup validation. It never closes the caller's profile handle.
// An OS lock protects each import/analysis/recovery even across processes.
type InstanceManager struct {
	profile   *gorm.DB
	ctx       context.Context
	cancel    context.CancelFunc
	root      string
	mu        sync.Mutex
	closed    bool
	closeDone chan struct{}
	ops       sync.WaitGroup
	instances map[string]*Database
	startup   []ValidationResult
}

type Database struct {
	ID      string
	manager *InstanceManager
	db      *gorm.DB
	path    string
	mu      sync.RWMutex
	closed  bool
	// Protected by manager.mu. Native Go callers retain their explicit owner;
	// Yak handles borrow this pool and release it after the final borrower.
	nativeOwner bool
	leases      int
}

func NewInstanceManager(profile *gorm.DB, libraryDir string) (*InstanceManager, error) {
	if profile == nil {
		return nil, fmt.Errorf("pcapdb: profile database is required")
	}
	root, err := filepath.Abs(libraryDir)
	if err != nil {
		return nil, err
	}
	if err = makeDurableDirectory(filepath.Join(root, ".locks")); err != nil {
		return nil, err
	}
	if err = migratePCAPMetadata(profile); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &InstanceManager{profile: profile, ctx: ctx, cancel: cancel, root: root, instances: make(map[string]*Database), closeDone: make(chan struct{})}
	if err = m.recoverCatalog(); err != nil {
		cancel()
		return nil, err
	}
	entries, err := m.List()
	if err != nil {
		cancel()
		return nil, err
	}
	for _, entry := range entries {
		result, err := m.Validate(context.Background(), entry.DatasetID, false)
		if err != nil {
			cancel()
			return nil, err
		}
		m.startup = append(m.startup, *result)
	}
	return m, nil
}

// The profile DB is a catalog mirror and may have weaker durability settings.
// Recover committed descriptors from package-owned UUID directories if a catalog
// registration was lost. Never hash a large capture or repair an index at startup.
func (m *InstanceManager) recoverCatalog() error {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return err
	}
	var ids []string
	if err = m.profile.Unscoped().Model(&PCAPFileDBMetadata{}).Pluck("dataset_id", &ids).Error; err != nil {
		return err
	}
	registered := make(map[string]bool, len(ids))
	for _, id := range ids {
		registered[id] = true
	}
	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() || registered[id] {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		path := filepath.Join(m.root, id, "index.sqlite")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		probe := &PCAPFileDBMetadata{DatasetID: id, DatabasePath: path}
		lock, err := acquireFileLock(m.ctx, datasetLockPath(probe), false)
		if errors.Is(err, ErrBusy) {
			continue
		}
		if err != nil {
			return err
		}
		recoverOne := func() error {
			var found PCAPFileDBMetadata
			if err := m.profile.Where("dataset_id = ?", id).First(&found).Error; err == nil {
				return nil
			} else if !gorm.IsRecordNotFoundError(err) {
				return err
			}
			reader, err := openIndex(path, false, false)
			if err != nil {
				m.startup = append(m.startup, ValidationResult{Metadata: *probe, Error: err.Error()})
				return nil
			}
			meta, err := readManifest(m.ctx, reader, id)
			reader.Close()
			if err != nil {
				m.startup = append(m.startup, ValidationResult{Metadata: *probe, Error: err.Error()})
				return nil
			}
			if meta.DatabasePath != path {
				// A closed self-contained child can be moved with its UUID directory.
				// Rebind only this package-owned location, under its writer lock.
				meta.DatabasePath = path
				writer, err := openIndex(path, true, false)
				if err != nil {
					return err
				}
				err = writeManifest(m.ctx, writer, meta)
				writer.Close()
				if err != nil {
					return err
				}
			}
			if meta.FullSHA256 != "" {
				if same, err := m.findContent(meta.FullSHA256); err != nil {
					return err
				} else if same != nil {
					return nil
				}
			}
			return m.saveCatalog(meta)
		}
		err = recoverOne()
		releaseFileLock(lock)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *InstanceManager) isClosed() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.closed }

func (m *InstanceManager) begin() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	m.ops.Add(1)
	return nil
}

func (m *InstanceManager) Close() error {
	m.mu.Lock()
	if m.closed {
		done := m.closeDone
		m.mu.Unlock()
		<-done
		return nil
	}
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	defer close(m.closeDone)
	m.ops.Wait()
	m.mu.Lock()
	instances := make([]*Database, 0, len(m.instances))
	for _, db := range m.instances {
		instances = append(instances, db)
	}
	m.mu.Unlock()
	var err error
	for _, db := range instances {
		err = errors.Join(err, db.close(true))
	}
	return err
}

func (m *InstanceManager) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(m.ctx, cancel)
	if m.ctx.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

func (m *InstanceManager) StartupValidation() []ValidationResult {
	return append([]ValidationResult(nil), m.startup...)
}

func (m *InstanceManager) List() ([]PCAPFileDBMetadata, error) {
	return m.ListContext(context.Background())
}
func (m *InstanceManager) Count() (int64, error) {
	return m.CountContext(context.Background())
}

func (m *InstanceManager) Metadata(identifier string) (*PCAPFileDBMetadata, error) {
	return m.MetadataContext(context.Background(), identifier)
}

func (m *InstanceManager) MetadataContext(ctx context.Context, identifier string) (*PCAPFileDBMetadata, error) {
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	return m.resolveContext(ctx, identifier)
}

func (m *InstanceManager) resolve(identifier string) (*PCAPFileDBMetadata, error) {
	return m.resolveContext(m.ctx, identifier)
}

func (m *InstanceManager) resolveContext(ctx context.Context, identifier string) (*PCAPFileDBMetadata, error) {
	scoped := indexWithContext(ctx, m.profile)
	var meta PCAPFileDBMetadata
	err := scoped.Where("dataset_id = ?", identifier).First(&meta).Error
	if err == nil {
		return &meta, nil
	}
	if !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	path, err := filepath.Abs(identifier)
	if err != nil {
		return nil, err
	}
	var matches []PCAPFileDBMetadata
	err = scoped.Where("source_path = ? OR database_path = ? OR EXISTS (SELECT 1 FROM json_each(source_aliases) WHERE value = ?)", path, path, path).Limit(2).Find(&matches).Error
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	if len(matches) != 1 {
		return nil, ErrAmbiguous
	}
	return &matches[0], nil
}

func datasetLockPath(meta *PCAPFileDBMetadata) string {
	return filepath.Join(filepath.Dir(meta.DatabasePath), ".instance.lock")
}
func (m *InstanceManager) saveCatalog(meta *PCAPFileDBMetadata) error {
	var existing PCAPFileDBMetadata
	err := m.profile.Unscoped().Where("dataset_id = ?", meta.DatasetID).First(&existing).Error
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return err
	}
	// The manifest may carry a row ID from an earlier profile database. Resolve
	// the current catalog row by the stable dataset ID before writing it.
	meta.Model.ID = existing.Model.ID
	if gorm.IsRecordNotFoundError(err) {
		return m.profile.Create(meta).Error
	}
	return m.profile.Save(meta).Error
}

func (m *InstanceManager) checkpoint(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata) error {
	meta.UpdatedAt = time.Now().UTC()
	if err := writeManifest(ctx, db, meta); err != nil {
		return err
	}
	return m.saveCatalog(meta)
}

func (m *InstanceManager) transition(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata, to State) error {
	if !allowedTransition(meta.State, to) {
		return fmt.Errorf("pcapdb: invalid state transition %s -> %s", meta.State, to)
	}
	meta.State, meta.LastError = to, ""
	return m.checkpoint(ctx, db, meta)
}

// Validate's fast mode checks identity, schema and the durable checkpoint;
// deep mode also hashes all capture/message BLOBs and runs SQLite quick_check. Invalid entries
// remain registered for inspection. No missing/corrupt file is recreated here.
func (m *InstanceManager) Validate(ctx context.Context, identifier string, deep bool) (*ValidationResult, error) {
	return m.validate(ctx, identifier, deep, false)
}

func (m *InstanceManager) validate(ctx context.Context, identifier string, deep, wait bool) (*ValidationResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("pcapdb: nil validation context")
	}
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	meta, err := m.resolveContext(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if _, err = uuid.Parse(meta.DatasetID); err != nil {
		return m.invalidCatalog(meta, fmt.Errorf("pcapdb: malformed dataset ID"))
	}
	if meta.DatabaseType != "sqlite" || !filepath.IsAbs(meta.DatabasePath) {
		return m.invalidCatalog(meta, fmt.Errorf("pcapdb: unsupported database type or non-absolute dataset paths"))
	}
	result := &ValidationResult{Metadata: *meta}
	lock, err := acquireFileLock(ctx, datasetLockPath(meta), wait)
	if errors.Is(err, ErrBusy) {
		result.Busy = true
		result.Error = err.Error()
		return result, nil
	}
	if err == nil {
		defer releaseFileLock(lock)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		// Without the OS lock we cannot write even an Invalid child checkpoint.
		// Keep the diagnostic in the catalog; a successful later validation
		// refreshes it from the authoritative child manifest.
		return m.invalidCatalog(meta, err)
	}
	return m.validateLocked(ctx, meta, deep)
}

func (m *InstanceManager) invalid(ctx context.Context, meta *PCAPFileDBMetadata, cause error) (*ValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	meta.State, meta.LastError, meta.ValidatedAt = StateInvalid, cause.Error(), time.Now().UTC()
	meta.UpdatedAt = meta.ValidatedAt
	// Also invalidate an intact manifest so already-open handles see the
	// change. Unsupported/corrupt/missing indexes are left untouched.
	if reader, err := openIndex(meta.DatabasePath, false, false); err == nil {
		stored, readErr := readManifest(ctx, reader, meta.DatasetID)
		reader.Close()
		if readErr == nil && stored.DatabasePath == meta.DatabasePath {
			writer, err := openIndex(meta.DatabasePath, true, false)
			if err != nil {
				return nil, err
			}
			err = writeManifest(ctx, writer, meta)
			writer.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return m.invalidCatalog(meta, cause)
}

func (m *InstanceManager) invalidCatalog(meta *PCAPFileDBMetadata, cause error) (*ValidationResult, error) {
	meta.State, meta.LastError, meta.ValidatedAt = StateInvalid, cause.Error(), time.Now().UTC()
	meta.UpdatedAt = meta.ValidatedAt
	if err := m.saveCatalog(meta); err != nil {
		return nil, err
	}
	return &ValidationResult{Metadata: *meta, Error: cause.Error()}, nil
}

func (m *InstanceManager) validateLocked(ctx context.Context, catalog *PCAPFileDBMetadata, deep bool) (*ValidationResult, error) {
	if catalog.DatabaseType != "sqlite" {
		return m.invalid(ctx, catalog, fmt.Errorf("pcapdb: unsupported database type %q", catalog.DatabaseType))
	}
	db, err := openIndex(catalog.DatabasePath, false, false)
	if err != nil {
		return m.invalid(ctx, catalog, err)
	}
	defer db.Close()
	meta, err := readManifest(ctx, db, catalog.DatasetID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return m.invalid(ctx, catalog, err)
	}
	if meta.DatabasePath != catalog.DatabasePath {
		return m.invalid(ctx, catalog, fmt.Errorf("pcapdb: manifest/catalog paths disagree"))
	}
	if activeState(meta.State) {
		// A live writer owns .instance.lock. Acquiring it proves that this work
		// was interrupted; a slow but live importer is never declared dead.
		// SQLite WAL recovery rolls uncommitted rows and their BLOBs back
		// together. There are no external byte files to truncate or repair.
		meta.State, meta.LastError = StateInterrupted, "operation interrupted before publication; retry import to rebuild"
		writer, err := openIndex(meta.DatabasePath, true, false)
		if err != nil {
			return m.invalid(ctx, catalog, err)
		}
		err = m.checkpoint(ctx, writer, meta)
		writer.Close()
		if err != nil {
			return nil, err
		}
	}
	if meta.State == StateReady {
		if deep {
			if err := validateIndexContent(ctx, db, meta); err != nil {
				return m.invalid(ctx, meta, err)
			}
			var integrity string
			if err = indexWithContext(ctx, db).Raw("PRAGMA quick_check").Row().Scan(&integrity); err != nil {
				return m.invalid(ctx, meta, err)
			}
			if integrity != "ok" {
				return m.invalid(ctx, meta, fmt.Errorf("pcapdb: index integrity: %s", integrity))
			}
			capture := newCaptureStoreReader(ctx, db, meta)
			hash := sha256.New()
			_, hashErr := copyContext(ctx, hash, capture)
			capture.Close()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if hashErr != nil {
				return m.invalid(ctx, meta, hashErr)
			}
			if hex.EncodeToString(hash.Sum(nil)) != meta.FullSHA256 {
				return m.invalid(ctx, meta, fmt.Errorf("pcapdb: capture BLOB hash mismatch"))
			}
			if meta.ProtocolsIndexed {
				digest, hashErr := hashBlobData(ctx, db, &PCAPProtocolMessage{})
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				if hashErr != nil {
					return m.invalid(ctx, meta, hashErr)
				}
				if digest != meta.ProtocolDataSHA256 {
					return m.invalid(ctx, meta, fmt.Errorf("pcapdb: protocol BLOB hash mismatch"))
				}
			}
			if meta.StreamsIndexed {
				digest, hashErr := hashBlobData(ctx, db, &PCAPStreamChunk{})
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				if hashErr != nil {
					return m.invalid(ctx, meta, hashErr)
				}
				if digest != meta.StreamDataSHA256 {
					return m.invalid(ctx, meta, fmt.Errorf("pcapdb: stream BLOB hash mismatch"))
				}
			}
		}
	}
	meta.ValidatedAt = time.Now().UTC()
	// Refresh the catalog from the authoritative committed checkpoint. The
	// validation timestamp itself does not require an index write/fsync.
	if err = m.saveCatalog(meta); err != nil {
		return nil, err
	}
	return &ValidationResult{Metadata: *meta, Valid: meta.State == StateReady, Error: meta.LastError}, nil
}

func validateIndexContent(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata) error {
	scoped := indexWithContext(ctx, db)
	for _, check := range []struct {
		model    interface{}
		expected int64
	}{
		{&Packet{}, meta.PacketCount}, {&PCAPProtocolMessage{}, meta.ProtocolCount}, {&PCAPCaptureRecord{}, meta.CaptureRecordCount},
		{&PCAPSession{}, meta.SessionCount}, {&PCAPStream{}, meta.StreamCount}, {&PCAPStreamChunk{}, meta.StreamChunkCount},
	} {
		var count int64
		if err := scoped.Model(check.model).Count(&count).Error; err != nil {
			return err
		}
		if count != check.expected {
			return fmt.Errorf("pcapdb: index counters disagree with checkpoint")
		}
	}
	var bad int64
	if err := scoped.Table("pragma_foreign_key_check").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: packet/session/stream references disagree with checkpoint")
	}
	checks := []struct {
		model     interface{}
		condition string
		args      []interface{}
	}{
		{&Packet{}, "captured_length < 0 OR captured_length > ? OR original_length < captured_length OR typeof(data) != 'blob' OR length(data) != captured_length", []interface{}{maxCaptureBlock}},
		{&PCAPProtocolMessage{}, "data_length < 0 OR data_length > ? OR typeof(data) != 'blob' OR length(data) != data_length", []interface{}{maxCaptureBlock}},
		{&PCAPCaptureRecord{}, "typeof(prefix) != 'blob' OR typeof(suffix) != 'blob' OR length(prefix) > ? OR length(suffix) > ?", []interface{}{maxCaptureBlock, maxCaptureBlock}},
		{&PCAPStreamChunk{}, "byte_offset < 0 OR length <= 0 OR length > ? OR typeof(data) != 'blob' OR length(data) != length", []interface{}{streamChunkBytes}},
	}
	for _, check := range checks {
		if err := scoped.Model(check.model).Where(check.condition, check.args...).Count(&bad).Error; err != nil {
			return err
		}
		if bad != 0 {
			return fmt.Errorf("pcapdb: invalid packet/protocol/stream BLOB")
		}
	}
	if err := scoped.Model(&Packet{}).Joins("LEFT JOIN capture_interfaces AS ci ON ci.id = packets.capture_interface_id").Where("ci.id IS NULL OR ci.section != packets.section OR ci.interface_id != packets.interface_id OR ci.link_type != packets.link_type").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: packet interface domains disagree")
	}
	// Every captured packet occurs exactly once in the export framing.
	var framed int64
	if err := scoped.Model(&PCAPCaptureRecord{}).Where("packet_id IS NOT NULL").Count(&framed).Error; err != nil {
		return err
	}
	if framed != meta.PacketCount {
		return fmt.Errorf("pcapdb: capture packet framing is incomplete")
	}
	for _, check := range []struct {
		model      interface{}
		expression string
		expected   int64
	}{
		{&PCAPProtocolMessage{}, "COALESCE(sum(length(data)),0)", meta.ProtocolDataSize},
		{&PCAPStreamChunk{}, "COALESCE(sum(length(data)),0)", meta.StreamDataSize},
	} {
		var size int64
		if err := scoped.Model(check.model).Select(check.expression).Row().Scan(&size); err != nil {
			return err
		}
		if size != check.expected {
			return fmt.Errorf("pcapdb: BLOB byte counts disagree with checkpoint")
		}
	}
	if err := validateStreamContent(ctx, db); err != nil {
		return err
	}
	return nil
}

func hashBlobData(ctx context.Context, db *gorm.DB, model interface{}) (string, error) {
	hash := sha256.New()
	var after uint
	for {
		rows, err := indexWithContext(ctx, db).Model(model).Select("id,CASE WHEN length(data) <= 16777216 THEN data END").Where("id > ?", after).Order("id").Limit(64).Rows()
		if err != nil {
			return "", err
		}
		count := 0
		for rows.Next() {
			var id uint
			var data []byte
			if err = rows.Scan(&id, &data); err != nil {
				break
			}
			if data == nil {
				err = fmt.Errorf("pcapdb: invalid derived BLOB")
				break
			}
			if id != after+1 {
				err = fmt.Errorf("pcapdb: missing derived BLOB")
				break
			}
			hash.Write(data)
			after = id
			count++
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return "", err
		}
		if count == 0 {
			return hex.EncodeToString(hash.Sum(nil)), nil
		}
	}
}

func (m *InstanceManager) Open(ctx context.Context, identifier string) (*Database, error) {
	return m.open(ctx, identifier, false)
}

func (m *InstanceManager) open(ctx context.Context, identifier string, leased bool) (*Database, error) {
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	result, err := m.validate(ctx, identifier, false, leased)
	if err != nil {
		return nil, err
	}
	if result.Busy {
		return nil, ErrBusy
	}
	if !result.Valid {
		return nil, fmt.Errorf("%w: %s: %s", ErrNotReady, result.Metadata.State, result.Error)
	}
	return m.openReady(&result.Metadata, leased)
}

func (m *InstanceManager) openReady(meta *PCAPFileDBMetadata, leased ...bool) (*Database, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if instance := m.instances[meta.DatasetID]; instance != nil {
		if len(leased) == 0 || !leased[0] {
			instance.nativeOwner = true
		}
		return instance, nil
	}
	// Handles are explicit resources. Avoid an unbounded pool for AI loops
	// over many captures; Database.Close releases a slot.
	if len(m.instances) >= 32 {
		return nil, fmt.Errorf("pcapdb: 32 open datasets; close unused database handles")
	}
	db, err := openIndex(meta.DatabasePath, false, false)
	if err != nil {
		return nil, err
	}
	instance := &Database{ID: meta.DatasetID, manager: m, db: db, path: meta.DatabasePath}
	instance.nativeOwner = len(leased) == 0 || !leased[0]
	m.instances[meta.DatasetID] = instance
	return instance, nil
}

// Late field-index builds update planner statistics. Already opened SQLite
// connections can retain older statistics; replace the cached read pool while
// preserving the Database object and waiting for its current queries to finish.
func (m *InstanceManager) refreshReader(ctx context.Context, meta *PCAPFileDBMetadata) error {
	m.mu.Lock()
	instance := m.instances[meta.DatasetID]
	m.mu.Unlock()
	if instance == nil {
		return nil
	}
	for !instance.mu.TryLock() {
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	defer instance.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if instance.closed {
		return nil
	}
	reader, err := openIndex(meta.DatabasePath, false, false)
	if err != nil {
		return err
	}
	previous := instance.db
	instance.db = reader
	return previous.Close()
}

func (d *Database) Close() error {
	return d.close(false)
}

func (d *Database) close(force bool) error {
	return d.closePool(force, false)
}

func (d *Database) closeIdle() error {
	return d.closePool(false, true)
}

func (d *Database) closePool(force, idleOnly bool) error {
	d.manager.mu.Lock()
	if idleOnly && (d.nativeOwner || d.leases > 0) {
		d.manager.mu.Unlock()
		return nil
	}
	if d.leases > 0 && !force {
		d.manager.mu.Unlock()
		return ErrBusy
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		d.manager.mu.Unlock()
		return nil
	}
	d.closed = true
	delete(d.manager.instances, d.ID)
	err := d.db.Close()
	d.mu.Unlock()
	d.manager.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	meta := &PCAPFileDBMetadata{DatabasePath: d.path}
	lock, lockErr := acquireFileLock(ctx, datasetLockPath(meta), false)
	if errors.Is(lockErr, ErrBusy) || errors.Is(lockErr, os.ErrNotExist) {
		return err
	}
	if lockErr != nil {
		return errors.Join(err, lockErr)
	}
	defer releaseFileLock(lock)
	writer, openErr := openIndex(d.path, true, false)
	if openErr != nil {
		return errors.Join(err, openErr)
	}
	checkpointErr := checkpointIndex(ctx, writer)
	return errors.Join(err, checkpointErr, writer.Close())
}

func (d *Database) Metadata() (*PCAPFileDBMetadata, error) {
	d.mu.RLock()
	if d.closed {
		d.mu.RUnlock()
		return nil, ErrClosed
	}
	d.mu.RUnlock()
	return d.manager.Metadata(d.ID)
}

func (m *InstanceManager) addAlias(ctx context.Context, meta *PCAPFileDBMetadata, path string) error {
	if path == meta.SourcePath {
		return nil
	}
	var aliases []string
	if err := json.Unmarshal([]byte(meta.SourceAliases), &aliases); err != nil {
		return err
	}
	for _, alias := range aliases {
		if alias == path {
			return nil
		}
	}
	aliases = append(aliases, path)
	data, err := json.Marshal(aliases)
	if err != nil {
		return err
	}
	meta.SourceAliases = string(data)
	db, err := openIndex(meta.DatabasePath, true, false)
	if err != nil {
		return err
	}
	defer db.Close()
	return m.checkpoint(ctx, db, meta)
}
