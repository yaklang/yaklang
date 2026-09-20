package analyzer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func TestHandlerParsedResultBudget(t *testing.T) {
	l, err := (budget.Limits{MaxResultBytes: 256}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	ctx := budget.Bind(context.Background(), l)
	libs := make(types.Libraries, 40)
	for i := range libs {
		libs[i] = types.Library{Name: fmt.Sprintf("pkg-%d", i), Version: "1.0.0", ID: fmt.Sprintf("id-%d", i)}
	}
	_, err = handlerParsedBudget(ctx, libs, nil)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("merge DTO created without result-memory precheck: %v", err)
	}
}
