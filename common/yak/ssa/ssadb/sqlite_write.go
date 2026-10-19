package ssadb

// WithSQLiteWriteLock used to serialize every SSA cache write behind a
// process-wide mutex because SQLite has a single writer per database file.
// The SSA cache DSN already sets _txlock=immediate and busy_timeout=10000
// (see consts.ssaSQLiteTxLock / the DSN builder), so SQLite itself queues
// concurrent writers and retries on lock contention. The extra mutex only
// turned every parallel compile unit back into a serial one: run
// 2026-10-18-2316 showed 652/655 goroutines parked on this lock while a
// single commit ran. Keep the helper so callers stay dialect-aware, but let
// the database arbitrate.
func WithSQLiteWriteLock(dialect string, work func() error) error {
	return work()
}
