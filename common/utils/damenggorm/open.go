package damenggorm

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/yaklang/gorm/dialects/dameng"
)

var schemaName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

func OpenDB(dsn string) (*sql.DB, error) { return OpenDBContext(context.Background(), dsn) }

// OpenDBContext applies SSA's schema and field-label policy, then uses the
// generic cancellation-safe connection pool. No schema is created here.
func OpenDBContext(ctx context.Context, dsn string) (*sql.DB, error) {
	prepared, err := prepareDSN(dsn)
	if err != nil {
		return nil, err
	}
	return dameng.OpenDBContext(ctx, prepared)
}

func prepareDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != Dialect || u.Host == "" || !schemaName.MatchString(u.Query().Get("schema")) {
		return "", errors.New("DM SSA requires an explicit uppercase schema")
	}
	query := u.Query()
	requestedSchema := query.Get("schema")
	for key := range query {
		// The SDK matches property names without case sensitivity. Remove all
		// variants so caller options cannot override SSA's field-label policy.
		if strings.EqualFold(key, "columnNameCase") || strings.EqualFold(key, "schema") {
			query.Del(key)
		}
	}
	query.Set("schema", requestedSchema)
	query.Set("columnNameCase", "lower")
	u.RawQuery = query.Encode()
	return u.String(), nil
}
