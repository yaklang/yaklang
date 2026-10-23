package consts

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type dmDatabasePool struct {
	maxOpen     int
	maxIdle     int
	maxLifetime time.Duration
}

func defaultDMDatabasePool() dmDatabasePool {
	return dmDatabasePool{maxOpen: 4, maxIdle: 2, maxLifetime: 30 * time.Minute}
}

// These are application settings, not DM protocol properties. Parse them before
// opening the driver, and never include the URL or a supplied value in errors.
func parseDMDatabasePool(raw string) (string, dmDatabasePool, error) {
	invalid := errors.New("invalid DM connection pool options")
	u, err := url.Parse(raw)
	if err != nil {
		return "", dmDatabasePool{}, invalid
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", dmDatabasePool{}, invalid
	}
	pool := defaultDMDatabasePool()
	keys := []string{"poolMaxOpenConns", "poolMaxIdleConns", "poolMaxLifetimeSeconds"}
	values := []int{pool.maxOpen, pool.maxIdle, int(pool.maxLifetime / time.Second)}
	explicitIdle := false
	for key, items := range query {
		if !strings.HasPrefix(strings.ToLower(key), "pool") {
			continue
		}
		known := false
		for i, name := range keys {
			if !strings.EqualFold(key, name) {
				continue
			}
			known = true
			if key != name || len(items) != 1 {
				return "", dmDatabasePool{}, invalid
			}
			if items[0] == "" || strings.IndexFunc(items[0], func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return "", dmDatabasePool{}, invalid
			}
			value, err := strconv.Atoi(items[0])
			if err != nil {
				return "", dmDatabasePool{}, invalid
			}
			values[i] = value
			explicitIdle = explicitIdle || i == 1
			query.Del(key)
		}
		if !known {
			return "", dmDatabasePool{}, invalid
		}
	}
	if !explicitIdle && values[1] > values[0] {
		values[1] = values[0]
	}
	if values[0] < 1 || values[0] > 1024 || values[1] < 0 || values[1] > values[0] || values[2] < 1 || values[2] > 86400 {
		return "", dmDatabasePool{}, invalid
	}
	pool = dmDatabasePool{maxOpen: values[0], maxIdle: values[1], maxLifetime: time.Duration(values[2]) * time.Second}
	u.RawQuery = query.Encode()
	return u.String(), pool, nil
}

func (pool dmDatabasePool) apply(db *sql.DB) {
	db.SetMaxOpenConns(pool.maxOpen)
	db.SetMaxIdleConns(pool.maxIdle)
	db.SetConnMaxLifetime(pool.maxLifetime)
}

func openDMSQLDatabase(raw string, open func(string) (*sql.DB, error)) (*sql.DB, error) {
	dsn, pool, err := parseDMDatabasePool(raw)
	if err != nil {
		return nil, err
	}
	db, err := open(dsn)
	if err != nil {
		return nil, err
	}
	pool.apply(db)
	return db, nil
}
