package textdecode

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

// ReserveRecordConversion is the allowance for the fixed Cargo, Poetry and
// pnpm record conversions after their bounded grammar reader. Per value, 4KiB
// covers DTOs, three identity indexes, their geometric backing growth and
// Library/Dependency copies. 24 times string bytes covers JSON escaping and
// simultaneously live origin, digest and native-ID copies. It is not a generic
// reflection decoder budget: only these readers' finite value types are valid.
func ReserveRecordConversion(ctx context.Context, value any) error {
	ctx = budget.Ensure(ctx)
	st := budget.From(ctx)
	var visit func(any, int) error
	visit = func(v any, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > st.Limits.MaxSyntaxDepth {
			return scanerr.New(scanerr.ResourceLimit, "record conversion depth")
		}
		if err := st.Working(4096); err != nil {
			return err
		}
		chargeString := func(s string) error {
			n, err := budget.SizeMul(len(s), 24)
			if err != nil {
				return err
			}
			return st.Working(n)
		}
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if err := chargeString(k); err != nil {
					return err
				}
				if err := visit(e, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, e := range x {
				if err := visit(e, depth+1); err != nil {
					return err
				}
			}
		case string:
			return chargeString(x)
		case nil, bool, int, int64, float64:
		default:
			return scanerr.New(scanerr.MalformedInput, "unexpected fixed record value")
		}
		return nil
	}
	return visit(value, 0)
}
