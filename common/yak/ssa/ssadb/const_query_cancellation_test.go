package ssadb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConstSearchHonorsRuleDeadlineWithoutRetry(t *testing.T) {
	db := setupA3ConstDB(t)
	cache := NewNameCache("deadline-test", false)
	// Occupy the only DB connection: the search must respect its caller's
	// deadline while waiting, not start an unbounded fallback query.
	conn, err := db.DB().Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var reported error
	ctx = WithQueryErrorHandler(ctx, func(err error) { reported = err })
	start := time.Now()
	before := NativeConstTypeIDQueries()
	ch := SearchVariable(db, ctx, "deadline-test", cache, RegexpCompare, ConstType, "user.*")
	for range ch {
		t.Fatal("a failed query must not yield matches")
	}
	require.ErrorIs(t, reported, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, before+1, NativeConstTypeIDQueries())
}

func TestConstSearchReportsDatabaseFailure(t *testing.T) {
	db := setupA3ConstDB(t)
	require.NoError(t, db.DropTable(&IrCode{}).Error)
	var reported error
	ctx := WithQueryErrorHandler(context.Background(), func(err error) { reported = err })
	for range SearchVariable(db, ctx, "missing-table", NewNameCache("missing-table", false), ExactCompare, ConstType, "value") {
		t.Fatal("unexpected match")
	}
	require.Error(t, reported, "SQL failure must be distinguishable from zero matches")
}
