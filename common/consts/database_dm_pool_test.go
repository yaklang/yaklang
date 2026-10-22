package consts

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
)

const dmPoolTestURL = "dm://app:private-password@db.example:5236?schema=SSA_IR"

func TestDMDatabasePoolOptionsAndDriverURL(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		open, idle  int
		lifetime    time.Duration
	}{
		{"defaults", "", 4, 2, 30 * time.Minute},
		{"one connection clamps default idle", "&poolMaxOpenConns=1", 1, 1, 30 * time.Minute},
		{"explicit pool", "&poolMaxOpenConns=7&poolMaxIdleConns=3&poolMaxLifetimeSeconds=45", 7, 3, 45 * time.Second},
		{"no idle connections", "&poolMaxIdleConns=0", 4, 0, 30 * time.Minute},
		{"minimum bounds", "&poolMaxOpenConns=1&poolMaxIdleConns=0&poolMaxLifetimeSeconds=1", 1, 0, time.Second},
		{"leading zeros", "&poolMaxOpenConns=0002&poolMaxIdleConns=00&poolMaxLifetimeSeconds=001", 2, 0, time.Second},
		{"maximum bounds", "&poolMaxOpenConns=1024&poolMaxIdleConns=1024&poolMaxLifetimeSeconds=86400", 1024, 1024, 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := dmPoolTestURL + "&columnNameCase=lower&appName=ssa%20ir" + tc.query
			dsn, pool, err := parseDMDatabasePool(raw)
			require.NoError(t, err)
			require.Equal(t, dmDatabasePool{maxOpen: tc.open, maxIdle: tc.idle, maxLifetime: tc.lifetime}, pool)
			u, err := url.Parse(dsn)
			require.NoError(t, err)
			require.Equal(t, "SSA_IR", u.Query().Get("schema"))
			require.Equal(t, "lower", u.Query().Get("columnNameCase"))
			require.Equal(t, "ssa ir", u.Query().Get("appName"))
			require.Len(t, u.Query(), 3, "application pool options must not reach GORM/SDK")
			original, err := url.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, original.User.String(), u.User.String())
			require.Equal(t, original.Host, u.Host)
		})
	}
}

func TestDMDatabasePoolInvalidOptionsFailBeforeDriver(t *testing.T) {
	for _, query := range []string{
		"&poolMaxOpenConns=0", "&poolMaxOpenConns=1025", "&poolMaxOpenConns=-1", "&poolMaxOpenConns=+1",
		"&poolMaxOpenConns=1.5", "&poolMaxOpenConns=", "&poolMaxOpenConns=999999999999999999999999",
		"&poolMaxIdleConns=-1", "&poolMaxIdleConns=5", "&poolMaxOpenConns=1&poolMaxIdleConns=2",
		"&poolMaxLifetimeSeconds=0", "&poolMaxLifetimeSeconds=86401", "&poolMaxLifetimeSeconds=private-password",
		"&poolMaxOpenConns=2&poolMaxOpenConns=2", "&poolMaxIdleConns=1&poolMaxIdleConns=1",
		"&poolMaxLifetimeSeconds=1&poolMaxLifetimeSeconds=2", "&PoolMaxOpenConns=2", "&poolmaxidleconns=1",
		"&POOLMAXLIFETIMESECONDS=1", "&poolMaxOpenConns=2&POOLMAXOPENCONNS=3", "&bad=%zz",
		"&poolMaxOpenConnections=1", "&POOLunknown=1", "&pool=1", "&poolMaxOpenConns=２", "&poolMaxIdleConns=%200",
	} {
		t.Run(query, func(t *testing.T) {
			db, err := openDMSQLDatabase(dmPoolTestURL+query, func(string) (*sql.DB, error) {
				t.Fatal("invalid pool options must not reach the driver")
				return nil, nil
			})
			require.Nil(t, db)
			require.EqualError(t, err, "invalid DM connection pool options")
			// Exercise the production entrypoint with either driver build tag.
			orm, err := createAndConfigDatabase(dmPoolTestURL+query, damenggorm.Dialect)
			require.Nil(t, orm)
			require.EqualError(t, err, "invalid DM connection pool options")
		})
	}
	_, _, err := parseDMDatabasePool("dm://app:private-password%zz@db.example:5236?schema=SSA_IR")
	require.EqualError(t, err, "invalid DM connection pool options")
}

func openPoolTestDatabase(t *testing.T, dialect, raw string) *gorm.DB {
	t.Helper()
	RegisterDriverOnce.Do(registerDriver)
	sqlDB, err := openDMSQLDatabase(raw, func(dsn string) (*sql.DB, error) {
		parsed, parseErr := url.Parse(dsn)
		require.NoError(t, parseErr)
		require.Len(t, parsed.Query(), 1, "factory must strip every pool option before driver open")
		require.Equal(t, "SSA_IR", parsed.Query().Get("schema"))
		return sql.Open(SQLiteExtend, filepath.Join(t.TempDir(), "pool.db"))
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	orm, err := gorm.Open(dialect, sqlDB)
	require.NoError(t, err)
	return orm
}

func TestDMDatabasePoolSurvivesSSAReconfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		open, idle  int
	}{
		{"default", "", 4, 2},
		{"override", "&poolMaxOpenConns=3&poolMaxIdleConns=1", 3, 1},
		{"one open", "&poolMaxOpenConns=1", 1, 1},
		{"zero idle", "&poolMaxOpenConns=2&poolMaxIdleConns=0", 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openPoolTestDatabase(t, damenggorm.ORMName, dmPoolTestURL+tc.query)
			// Match factory initialization followed by SSA's post-migration pass.
			configureAndOptimizeDBWithOptions(damenggorm.Dialect, db, defaultDatabaseOpenOptions())
			configureAndOptimizeDBWithOptions(damenggorm.Dialect, db, ssaDatabaseOpenOptions())
			configureAndOptimizeDB(damenggorm.ORMName, db)
			require.Equal(t, tc.open, db.DB().Stats().MaxOpenConnections)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var conns []*sql.Conn
			for i := 0; i < tc.open; i++ {
				conn, err := db.DB().Conn(ctx)
				require.NoError(t, err)
				conns = append(conns, conn)
			}
			for _, conn := range conns {
				require.NoError(t, conn.Close())
			}
			require.Equal(t, tc.idle, db.DB().Stats().Idle, "SSA reconfiguration must preserve the idle limit")
		})
	}
}

func TestDMDatabasePoolLifetimeSurvivesSSAReconfiguration(t *testing.T) {
	db := openPoolTestDatabase(t, damenggorm.ORMName, dmPoolTestURL+"&poolMaxOpenConns=1&poolMaxLifetimeSeconds=1")
	configureAndOptimizeDBWithOptions(damenggorm.Dialect, db, ssaDatabaseOpenOptions())
	require.Eventually(t, func() bool {
		require.NoError(t, db.DB().Ping())
		return db.DB().Stats().MaxLifetimeClosed > 0
	}, 5*time.Second, 50*time.Millisecond, "SSA second pass must not reset the selected 1s lifetime to 1h")
}

func TestDMDatabasePoolUnboundedWrapperAndPostgresCompatibility(t *testing.T) {
	for _, dialect := range []string{damenggorm.Dialect, Postgres} {
		t.Run(dialect, func(t *testing.T) {
			RegisterDriverOnce.Do(registerDriver)
			sqlDB, err := sql.Open(SQLiteExtend, filepath.Join(t.TempDir(), "pool.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(dialect, sqlDB)
			require.NoError(t, err)
			require.Zero(t, sqlDB.Stats().MaxOpenConnections)
			configureAndOptimizeDB(dialect, db)
			want := 4
			if dialect == Postgres {
				want = 20
			}
			require.Equal(t, want, sqlDB.Stats().MaxOpenConnections)
		})
	}
	pgURL := "postgres://app:private-password@db.example/ir?sslmode=disable&poolMaxOpenConns=3"
	dialect, raw := parseDatabaseURL(pgURL)
	require.Equal(t, Postgres, dialect)
	require.Equal(t, pgURL, raw, "PG URL handling must remain unchanged")
}
