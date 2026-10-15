package pcapdb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
)

var defaultInstances struct {
	sync.Mutex
	manager *InstanceManager
}

// DefaultManager is lazy: importing the Yak module does not open databases or
// walk the capture library. Each manager's constructor validates its catalog.
func DefaultManager() (*InstanceManager, error) {
	defaultInstances.Lock()
	profile := consts.GetGormProfileDatabase()
	if profile == nil {
		defaultInstances.Unlock()
		return nil, fmt.Errorf("pcapdb: profile database unavailable")
	}
	if defaultInstances.manager != nil && defaultInstances.manager.profile.DB() == profile.DB() && !defaultInstances.manager.isClosed() {
		manager := defaultInstances.manager
		defaultInstances.Unlock()
		return manager, nil
	}
	manager, err := NewInstanceManager(profile, filepath.Join(consts.GetDefaultYakitBaseDir(), "pcap-library"))
	if err != nil {
		defaultInstances.Unlock()
		return nil, err
	}
	previous := defaultInstances.manager
	defaultInstances.manager = manager
	defaultInstances.Unlock()
	if previous != nil {
		// A progress callback on the previous manager may itself be resolving
		// the new default. Do not wait for that import on this same call stack.
		go previous.Close()
	}
	return manager, nil
}

func GetOrCreatePCAPDatabase(filename string, options ...ImportOption) (*Database, error) {
	manager, err := DefaultManager()
	if err != nil {
		return nil, err
	}
	return manager.GetOrCreate(filename, options...)
}

func RebuildPCAPDatabase(identifier string, options ...ImportOption) (*Database, error) {
	manager, err := DefaultManager()
	if err != nil {
		return nil, err
	}
	return manager.RebuildAnalysis(identifier, options...)
}

func ExportFromPCAPDatabase(identifier string, outputs ...string) (string, error) {
	return exportFromPCAPDatabase(context.Background(), identifier, outputs...)
}

func exportFromPCAPDatabase(ctx context.Context, identifier string, outputs ...string) (string, error) {
	if len(outputs) > 1 {
		return "", fmt.Errorf("pcapdb: at most one export output is allowed")
	}
	manager, err := DefaultManager()
	if err != nil {
		return "", err
	}
	meta, err := manager.Metadata(identifier)
	if err != nil {
		return "", err
	}
	output := meta.SourcePath + ".export." + meta.Format
	if len(outputs) > 0 {
		output = outputs[0]
	}
	return manager.Export(ctx, meta.DatasetID, output)
}

func ListPCAPDatabases() ([]PCAPFileDBMetadata, error) {
	manager, err := DefaultManager()
	if err != nil {
		return nil, err
	}
	return manager.List()
}
func CountPCAPDatabases() (int64, error) {
	manager, err := DefaultManager()
	if err != nil {
		return 0, err
	}
	return manager.Count()
}
func OpenPCAPDatabase(identifier string) (*Database, error) {
	manager, err := DefaultManager()
	if err != nil {
		return nil, err
	}
	return manager.Open(context.Background(), identifier)
}
func ValidatePCAPDatabase(identifier string, deep ...bool) (*ValidationResult, error) {
	return validatePCAPDatabase(context.Background(), identifier, deep...)
}

func validatePCAPDatabase(ctx context.Context, identifier string, deep ...bool) (*ValidationResult, error) {
	if len(deep) > 1 {
		return nil, fmt.Errorf("pcapdb: at most one deep-validation flag is allowed")
	}
	manager, err := DefaultManager()
	if err != nil {
		return nil, err
	}
	return manager.Validate(ctx, identifier, len(deep) > 0 && deep[0])
}

// ExportsWithContext binds imports, queries and handle leases to one Yak task.
func ExportsWithContext(ctx context.Context) map[string]any {
	return exportsWithFactory(ctx, DefaultManager)
}

// ExportsWithManager exposes identical per-task leases in an isolated library.
// Closing a task's module releases its leases, never another task's manager.
func ExportsWithManager(ctx context.Context, manager *InstanceManager) map[string]any {
	return exportsWithFactory(ctx, func() (*InstanceManager, error) { return manager, nil })
}

func exportsWithFactory(parent context.Context, factory func() (*InstanceManager, error)) map[string]any {
	exports := make(map[string]any, len(Exports))
	for key, value := range Exports {
		exports[key] = value
	}
	ctx, cancel := context.WithCancel(parent)
	var mu sync.Mutex
	handles := make(map[*DatabaseHandle]struct{})
	closed := false
	register := func(handle *DatabaseHandle) (*DatabaseHandle, error) {
		mu.Lock()
		if closed {
			mu.Unlock()
			_ = handle.Close()
			return nil, ErrClosed
		}
		handles[handle] = struct{}{}
		handle.mu.Lock()
		handle.onClose = func() { mu.Lock(); delete(handles, handle); mu.Unlock() }
		if handle.closed.Load() {
			delete(handles, handle)
		}
		handle.mu.Unlock()
		mu.Unlock()
		return handle, nil
	}
	attach := func(manager *InstanceManager, database *Database) (*DatabaseHandle, error) {
		handle, err := manager.acquire(ctx, database)
		if err != nil {
			return nil, err
		}
		return register(handle)
	}
	importOption := func(c *importConfig) error { c.parent = ctx; c.leased = true; return nil }
	exports["GetOrCreatePCAPDatabase"] = func(filename string, options ...ImportOption) (*DatabaseHandle, error) {
		manager, err := factory()
		if err != nil {
			return nil, err
		}
		database, err := manager.GetOrCreate(filename, append(options, importOption)...)
		if err != nil {
			return nil, err
		}
		return attach(manager, database)
	}
	exports["RebuildPCAPDatabase"] = func(identifier string, options ...ImportOption) (*DatabaseHandle, error) {
		manager, err := factory()
		if err != nil {
			return nil, err
		}
		database, err := manager.RebuildAnalysis(identifier, append(options, importOption)...)
		if err != nil {
			return nil, err
		}
		return attach(manager, database)
	}
	exports["OpenPCAPDatabase"] = func(identifier string) (*DatabaseHandle, error) {
		manager, err := factory()
		if err != nil {
			return nil, err
		}
		handle, err := manager.Acquire(ctx, identifier)
		if err != nil {
			return nil, err
		}
		return register(handle)
	}
	exports["ExportFromPCAPDatabase"] = func(identifier string, outputs ...string) (string, error) {
		if len(outputs) > 1 {
			return "", fmt.Errorf("pcapdb: at most one export output is allowed")
		}
		manager, err := factory()
		if err != nil {
			return "", err
		}
		meta, err := manager.MetadataContext(ctx, identifier)
		if err != nil {
			return "", err
		}
		output := meta.SourcePath + ".export." + meta.Format
		if len(outputs) > 0 {
			output = outputs[0]
		}
		return manager.Export(ctx, meta.DatasetID, output)
	}
	exports["ValidatePCAPDatabase"] = func(identifier string, deep ...bool) (*ValidationResult, error) {
		if len(deep) > 1 {
			return nil, fmt.Errorf("pcapdb: at most one deep-validation flag is allowed")
		}
		manager, err := factory()
		if err != nil {
			return nil, err
		}
		return manager.Validate(ctx, identifier, len(deep) > 0 && deep[0])
	}
	exports["ListPCAPDatabases"] = func() ([]PCAPFileDBMetadata, error) {
		manager, err := factory()
		if err != nil {
			return nil, err
		}
		return manager.ListContext(ctx)
	}
	exports["CountPCAPDatabases"] = func() (int64, error) {
		manager, err := factory()
		if err != nil {
			return 0, err
		}
		return manager.CountContext(ctx)
	}
	exports["ListPCAPDatabasesPage"] = func(options ...QueryOption) (*ResultPage[CatalogSummary], error) {
		manager, err := factory()
		if err != nil {
			return failPage(emptyPage[CatalogSummary]("", 0, "catalog_id"), err)
		}
		return manager.ListPage(ctx, options...)
	}
	exports["EnsureProtocolFieldIndexes"] = func(identifier string, paths ...string) ([]ProtocolFieldIndex, error) {
		manager, err := factory()
		if err != nil {
			return nil, err
		}
		return manager.EnsureProtocolFieldIndexes(ctx, identifier, paths...)
	}
	exports["ClosePCAPDatabases"] = func() error {
		mu.Lock()
		closed = true
		leases := make([]*DatabaseHandle, 0, len(handles))
		for h := range handles {
			leases = append(leases, h)
		}
		mu.Unlock()
		cancel()
		var err error
		for _, h := range leases {
			err = errors.Join(err, h.Close())
		}
		return err
	}
	return exports
}

func ClosePCAPDatabases() error {
	defaultInstances.Lock()
	manager := defaultInstances.manager
	defaultInstances.manager = nil
	defaultInstances.Unlock()
	if manager != nil {
		return manager.Close()
	}
	return nil
}

var Exports = map[string]any{
	"GetOrCreatePCAPDatabase":    GetOrCreatePCAPDatabase,
	"RebuildPCAPDatabase":        RebuildPCAPDatabase,
	"ExportFromPCAPDatabase":     ExportFromPCAPDatabase,
	"ListPCAPDatabases":          ListPCAPDatabases,
	"CountPCAPDatabases":         CountPCAPDatabases,
	"OpenPCAPDatabase":           OpenPCAPDatabase,
	"ValidatePCAPDatabase":       ValidatePCAPDatabase,
	"ClosePCAPDatabases":         ClosePCAPDatabases,
	"FingerprintFile":            FingerprintFile,
	"withContext":                WithContext,
	"resultBytes":                QueryResultBytes,
	"previewBytes":               QueryPreviewBytes,
	"EnsureProtocolFieldIndexes": EnsureProtocolFieldIndexes,
	"ListPCAPDatabasesPage":      ListPCAPDatabasesPage,
	"withBatchSize":              WithBatchSize,
	"withProtocols":              WithProtocols,
	"withStreams":                WithStreams,
	"withFieldIndex":             WithFieldIndex,
	"onProgress":                 WithProgress,
	"queryContext":               QueryContext,
	"limit":                      QueryLimit,
	"after":                      QueryAfter,
	"transport":                  QueryTransport,
	"protocol":                   QueryProtocol,
	"sourceIP":                   QuerySourceIP,
	"destinationIP":              QueryDestinationIP,
	"sourcePort":                 QuerySourcePort,
	"destinationPort":            QueryDestinationPort,
	"flow":                       QueryFlow,
	"session":                    QuerySession,
	"stream":                     QueryStream,
	"maxBytes":                   QueryMaxBytes,
	"timeRange":                  QueryTimeRange,
	"field":                      QueryField,
	"fieldExists":                QueryFieldExists,
	"fieldMissing":               QueryFieldMissing,
	"withFields":                 QueryWithFields,
}

func EnsureProtocolFieldIndexes(identifier string, paths ...string) ([]ProtocolFieldIndex, error) {
	manager, err := DefaultManager()
	if err != nil {
		return nil, err
	}
	return manager.EnsureProtocolFieldIndexes(context.Background(), identifier, paths...)
}
func ListPCAPDatabasesPage(options ...QueryOption) (*ResultPage[CatalogSummary], error) {
	manager, err := DefaultManager()
	if err != nil {
		return failPage(emptyPage[CatalogSummary]("", 0, "catalog_id"), err)
	}
	return manager.ListPage(context.Background(), options...)
}
