package minirehs

import (
	"errors"
	"fmt"
)

// Keep the native status, including invalid dispatch (-1), overflow (-2), and
// allocation failure (-3), until the owning database can attach the public ID.
type nativeExecutionError struct {
	stage string
	index int // -1 for a fused operation with no single responsible rule
	code  int
}

func (e *nativeExecutionError) Error() string {
	return fmt.Sprintf("native %s failed (index=%d code=%d)", e.stage, e.index, e.code)
}

func (d *mvsDB) nativeScanError(err error) error {
	var failure *nativeExecutionError
	if !errors.As(err, &failure) {
		return err
	}
	category := "execution"
	switch failure.code {
	case -1:
		category = "invalid_dispatch"
	case -2:
		category = "overflow"
	case -3:
		category = "out_of_memory"
	}
	code := failure.code
	result := &ScanError{Record: -1, Stage: "native_" + failure.stage, Category: category,
		NativeCode: &code, Incomplete: true, Err: err}
	if failure.index >= 0 && failure.index < len(d.all) {
		result.RuleID, result.HasRule = d.all[failure.index].id, true
	}
	return result
}

// The old private Boolean helpers are retained for differential tests. They
// must fail explicitly if used with a broken native execution plan.
func mustNativeMatch(hit bool, err error) bool {
	if err != nil {
		panic(err)
	}
	return hit
}

type nativeMergedResult struct {
	hits []int
	err  error
}

func nativeFailedBatch(n, code int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(code)
	}
	return out
}
