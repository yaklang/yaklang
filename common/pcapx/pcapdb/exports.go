package pcapdb

import (
	"context"
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

// ExportsWithContext binds long-running module calls to one Yak execution.
// Explicit import deadlines are combined with, not substituted for, this parent.
func ExportsWithContext(ctx context.Context) map[string]any {
	exports := make(map[string]any, len(Exports))
	for key, value := range Exports {
		exports[key] = value
	}
	importCapture := func(filename string, options ...ImportOption) (*Database, error) {
		parent := func(c *importConfig) error { c.parent = ctx; return nil }
		return GetOrCreatePCAPDatabase(filename, append(options, parent)...)
	}
	exports["GetOrCreatePCAPDatabase"] = importCapture
	exports["RebuildPCAPDatabase"] = func(identifier string, options ...ImportOption) (*Database, error) {
		parent := func(c *importConfig) error { c.parent = ctx; return nil }
		return RebuildPCAPDatabase(identifier, append(options, parent)...)
	}
	exports["ExportFromPCAPDatabase"] = func(identifier string, outputs ...string) (string, error) {
		return exportFromPCAPDatabase(ctx, identifier, outputs...)
	}
	exports["ValidatePCAPDatabase"] = func(identifier string, deep ...bool) (*ValidationResult, error) {
		return validatePCAPDatabase(ctx, identifier, deep...)
	}
	exports["OpenPCAPDatabase"] = func(identifier string) (*Database, error) {
		manager, err := DefaultManager()
		if err != nil {
			return nil, err
		}
		return manager.Open(ctx, identifier)
	}
	return exports
}

// ExportsWithManager embeds the same Yak API into an isolated library without
// replacing the process-wide profile/default manager (e.g. parallel mustpass).
// The caller owns the manager and its profile handle.
func ExportsWithManager(ctx context.Context, manager *InstanceManager) map[string]any {
	exports := make(map[string]any, len(Exports))
	for key, value := range Exports {
		exports[key] = value
	}
	exports["GetOrCreatePCAPDatabase"] = func(filename string, options ...ImportOption) (*Database, error) {
		parent := func(c *importConfig) error { c.parent = ctx; return nil }
		return manager.GetOrCreate(filename, append(options, parent)...)
	}
	exports["ExportFromPCAPDatabase"] = func(identifier string, outputs ...string) (string, error) {
		if len(outputs) > 1 {
			return "", fmt.Errorf("pcapdb: at most one export output is allowed")
		}
		meta, err := manager.Metadata(identifier)
		if err != nil {
			return "", err
		}
		output := meta.SourcePath + ".export." + meta.Format
		if len(outputs) > 0 {
			output = outputs[0]
		}
		return manager.Export(ctx, identifier, output)
	}
	exports["RebuildPCAPDatabase"] = func(identifier string, options ...ImportOption) (*Database, error) {
		parent := func(c *importConfig) error { c.parent = ctx; return nil }
		return manager.RebuildAnalysis(identifier, append(options, parent)...)
	}
	exports["ListPCAPDatabases"] = manager.List
	exports["CountPCAPDatabases"] = manager.Count
	exports["OpenPCAPDatabase"] = func(identifier string) (*Database, error) { return manager.Open(ctx, identifier) }
	exports["ValidatePCAPDatabase"] = func(identifier string, deep ...bool) (*ValidationResult, error) {
		if len(deep) > 1 {
			return nil, fmt.Errorf("pcapdb: at most one deep-validation flag is allowed")
		}
		return manager.Validate(ctx, identifier, len(deep) > 0 && deep[0])
	}
	exports["ClosePCAPDatabases"] = manager.Close
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
	"GetOrCreatePCAPDatabase": GetOrCreatePCAPDatabase,
	"RebuildPCAPDatabase":     RebuildPCAPDatabase,
	"ExportFromPCAPDatabase":  ExportFromPCAPDatabase,
	"ListPCAPDatabases":       ListPCAPDatabases,
	"CountPCAPDatabases":      CountPCAPDatabases,
	"OpenPCAPDatabase":        OpenPCAPDatabase,
	"ValidatePCAPDatabase":    ValidatePCAPDatabase,
	"ClosePCAPDatabases":      ClosePCAPDatabases,
	"FingerprintFile":         FingerprintFile,
	"withContext":             WithContext,
	"withBatchSize":           WithBatchSize,
	"withProtocols":           WithProtocols,
	"withStreams":             WithStreams,
	"withFieldIndex":          WithFieldIndex,
	"onProgress":              WithProgress,
	"queryContext":            QueryContext,
	"limit":                   QueryLimit,
	"after":                   QueryAfter,
	"transport":               QueryTransport,
	"protocol":                QueryProtocol,
	"sourceIP":                QuerySourceIP,
	"destinationIP":           QueryDestinationIP,
	"sourcePort":              QuerySourcePort,
	"destinationPort":         QueryDestinationPort,
	"flow":                    QueryFlow,
	"session":                 QuerySession,
	"stream":                  QueryStream,
	"maxBytes":                QueryMaxBytes,
	"timeRange":               QueryTimeRange,
	"field":                   QueryField,
	"fieldExists":             QueryFieldExists,
	"fieldMissing":            QueryFieldMissing,
	"withFields":              QueryWithFields,
}
