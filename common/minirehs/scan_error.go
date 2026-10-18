package minirehs

import (
	"errors"
	"fmt"
)

// ScanError reports an incomplete scan. Callbacks observed before the failure
// remain valid; callers must not interpret the returned prefix as a full result.
// Record is -1 for Scan, or the zero-based record index for ScanBatch. HasRule
// distinguishes an unknown rule from the valid PatternID zero. NativeCode is
// nil when the backend does not expose a code. Unwrap preserves errors.Is/As.
type ScanError struct {
	RuleID     PatternID
	HasRule    bool
	Record     int
	Stage      string
	NativeCode *int
	Incomplete bool
	Err        error
}

func (e *ScanError) Error() string {
	context := e.Stage
	if e.HasRule {
		context += fmt.Sprintf(" rule=%d", e.RuleID)
	}
	if e.Record >= 0 {
		context += fmt.Sprintf(" record=%d", e.Record)
	}
	if e.NativeCode != nil {
		context += fmt.Sprintf(" code=%d", *e.NativeCode)
	}
	return fmt.Sprintf("minirehs: incomplete scan (%s): %v", context, e.Err)
}

func (e *ScanError) Unwrap() error { return e.Err }

func scanError(err error) error {
	if err == nil {
		return nil
	}
	var existing *ScanError
	if errors.As(err, &existing) {
		return err
	}
	return &ScanError{Record: -1, Stage: "scan", Incomplete: true, Err: err}
}

func scanErrorAtRecord(err error, record int) error {
	if err == nil {
		return nil
	}
	var existing *ScanError
	if errors.As(scanError(err), &existing) {
		copy := *existing
		copy.Record = record
		if err != existing {
			copy.Err = err // retain context added by an outer wrapper
		}
		return &copy
	}
	return err
}
