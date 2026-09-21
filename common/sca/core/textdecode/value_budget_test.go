package textdecode

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"strings"
	"testing"
)

func TestRecordConversionReservation(t *testing.T) {
	record := map[string]any{"package": []any{map[string]any{"name": "x", "version": "1", "dependencies": []any{"y >=1"}}}}
	small := budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 1})
	if err := ReserveRecordConversion(small, record); !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal(err)
	}
	if err := ReserveRecordConversion(budget.Ensure(context.Background()), record); err != nil {
		t.Fatal(err)
	}
	large := map[string]any{"value": strings.Repeat("x", 10000)}
	if err := ReserveRecordConversion(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 100000}), large); !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ReserveRecordConversion(ctx, record); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
