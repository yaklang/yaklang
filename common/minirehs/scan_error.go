package minirehs

import (
	"errors"
	"fmt"

	pcrelite "github.com/VillanCh/go-pcre2-lite"
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
	Category   string
	NativeCode *int
	Incomplete bool
	Err        error
}

func verifierScanError(cp *compiledPattern, err error) error {
	if err == nil {
		return nil
	}
	backend := "pcre2"
	if cp.v.exact() {
		backend = "re2"
	}
	stage, category := "verify_"+backend, "execution"
	var initialization *verifierCompileError
	if errors.As(err, &initialization) {
		stage, category = "compile_"+backend, "compile"
	}
	switch {
	case errors.Is(err, pcrelite.ErrMatchLimit):
		category = "match_limit"
	case errors.Is(err, pcrelite.ErrDepthLimit):
		category = "depth_limit"
	case errors.Is(err, pcrelite.ErrBadUTF):
		category = "bad_utf"
	case errors.Is(err, pcrelite.ErrNoMemory):
		category = "out_of_memory"
	case errors.Is(err, pcrelite.ErrClosed):
		category = "closed"
	}
	var nativeCode *int
	var native *pcrelite.MatchError
	if errors.As(err, &native) {
		code := native.Code
		nativeCode = &code
	}
	// The binding exposes common limit/UTF failures as sentinels without a
	// raw code. Keep that field nil rather than inventing a numeric value.
	return &ScanError{RuleID: cp.id, HasRule: true, Record: -1, Stage: stage,
		Category: category, NativeCode: nativeCode, Incomplete: true, Err: err}
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
