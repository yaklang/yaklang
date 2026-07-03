package irschema

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.up.sql
var migrationsFS embed.FS

// Migration is a single versioned DDL file embedded in the binary.
//
// NoTx marks migrations that cannot run inside a transaction block (e.g.
// CREATE INDEX CONCURRENTLY). Phase 1 has no NoTx migrations; the field
// exists so future additive DDL is not forced to refactor Migrate's
// transaction model.
type Migration struct {
	Version  int64
	Checksum string // sha256(sql) hex; recorded in ir_schema_migrations and verified on every Check
	SQL      string
	NoTx     bool
}

// migrationFileNameRE matches NNNN_description.up.sql.
var migrationFileNameRE = regexp.MustCompile(`^(\d+)_.+\.up\.sql$`)

// EmbeddedMigrations returns all embedded migrations sorted by Version.
// The list is built once at first call and cached.
func EmbeddedMigrations() []Migration {
	if cachedMigrations == nil && cachedErr == nil {
		initOnce()
	}
	return cachedMigrations
}

// initOnce builds the cached migration list. We use a lazy init (not init())
// so a checksum failure surfaces at first use with a normal error path
// rather than a package-init panic.
var (
	cachedMigrations []Migration
	cachedErr        error
)

func initOnce() {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		cachedErr = fmt.Errorf("irschema: read migrations dir: %w", err)
		return
	}
	type raw struct {
		version int64
		name    string
	}
	var raws []raw
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFileNameRE.FindStringSubmatch(e.Name())
		if m == nil {
			cachedErr = fmt.Errorf("irschema: migration filename %q does not match NNNN_*.up.sql", e.Name())
			return
		}
		v, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			cachedErr = fmt.Errorf("irschema: parse version from %q: %w", e.Name(), err)
			return
		}
		raws = append(raws, raw{version: v, name: e.Name()})
	}
	sort.Slice(raws, func(i, j int) bool { return raws[i].version < raws[j].version })

	// Versions must be strictly contiguous starting at 1. A gap means a
	// developer forgot to bump the version constant after adding a file.
	out := make([]Migration, 0, len(raws))
	for i, r := range raws {
		if r.version != int64(i+1) {
			cachedErr = fmt.Errorf("irschema: migration versions must be contiguous starting at 1; got %d at index %d", r.version, i)
			return
		}
		data, err := migrationsFS.ReadFile("migrations/" + r.name)
		if err != nil {
			cachedErr = fmt.Errorf("irschema: read migration %q: %w", r.name, err)
			return
		}
		sum := sha256.Sum256(data)
		out = append(out, Migration{
			Version:  r.version,
			Checksum: hex.EncodeToString(sum[:]),
			SQL:      string(data),
			NoTx:     noTxFromName(r.name),
		})
	}
	cachedMigrations = out
}

func noTxFromName(name string) bool {
	// Convention: any migration whose filename contains "_notx_" runs
	// outside a transaction block (e.g. 0005_add_idx_concurrently_notx_*.up.sql).
	return strings.Contains(name, "_notx_")
}

// LookupMigration returns the migration for the given version, or nil if none.
func LookupMigration(version int64) *Migration {
	for _, m := range EmbeddedMigrations() {
		if m.Version == version {
			cp := m
			return &cp
		}
	}
	return nil
}

// MaxEmbeddedVersion returns the highest embedded migration version, or 0 if
// there are no embedded migrations (a configuration error).
func MaxEmbeddedVersion() int64 {
	ms := EmbeddedMigrations()
	if len(ms) == 0 {
		return 0
	}
	return ms[len(ms)-1].Version
}
