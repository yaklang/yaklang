package minirehs

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"testing"
)

var programExecutionFailure = errors.New("controlled execution failure")

type programFailingBackend struct{}

func (programFailingBackend) numAlwaysOn() int { return 0 }
func (programFailingBackend) close() error     { return nil }
func (programFailingBackend) scan(data []byte, _ *scratch, handler MatchHandler) (bool, error) {
	if len(data) == 0 {
		return false, nil
	}
	if data[0] != 'Z' && !handler(Match{ID: 17, From: 0, To: 1}) {
		return true, nil
	}
	if data[0] == 'E' || data[0] == 'Z' {
		return false, &ScanError{RuleID: 0, HasRule: true, Record: -1, Stage: "test-verifier", Incomplete: true, Err: programExecutionFailure}
	}
	return false, nil
}

func TestScanBatchErrorsRespectRecordAndEarlyStop(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(oldProcs)
	for _, procs := range []int{1, 2, 4} {
		runtime.GOMAXPROCS(procs)
		for _, failure := range []byte{'E', 'Z'} {
			for _, stopAt := range []int{-1, 0, 1} {
				t.Run(fmt.Sprintf("procs=%d/failure=%c/stop=%d", procs, failure, stopAt), func(t *testing.T) {
					db := &database{primary: programFailingBackend{}}
					records := make([][]byte, 17)
					for i := range records {
						records[i] = make([]byte, 4096)
						records[i][0] = 'A'
					}
					records[1][0] = failure
					var callbacks []int
					err := db.ScanBatch(records, nil, func(record int, _ Match) bool {
						callbacks = append(callbacks, record)
						return record != stopAt
					})
					want := []int{0}
					if stopAt != 0 && failure == 'E' {
						want = append(want, 1)
					}
					if !reflect.DeepEqual(callbacks, want) {
						t.Fatalf("callbacks=%v want=%v err=%v", callbacks, want, err)
					}
					stopped := stopAt == 0 || stopAt == 1 && failure == 'E'
					if stopped {
						if err != nil {
							t.Fatalf("early stop observed an unreachable later error: %v", err)
						}
						return
					}
					var failure *ScanError
					if !errors.Is(err, programExecutionFailure) || !errors.As(err, &failure) ||
						failure.Record != 1 || !failure.Incomplete || !failure.HasRule || failure.RuleID != 0 {
						t.Fatalf("lost record/rule/error context: %v", err)
					}
				})
			}
		}
	}
}

func TestScanErrorRecordAnnotationKeepsCause(t *testing.T) {
	code := -7
	original := &ScanError{RuleID: 0, HasRule: true, Record: -1, Stage: "native", NativeCode: &code, Incomplete: true, Err: programExecutionFailure}
	annotated := scanErrorAtRecord(original, 9)
	var got *ScanError
	if !errors.As(annotated, &got) || !errors.Is(annotated, programExecutionFailure) || got.Record != 9 ||
		got.NativeCode == nil || *got.NativeCode != code || original.Record != -1 {
		t.Fatalf("error annotation mutated or discarded the original: %v / %v", original, annotated)
	}
	outer := fmt.Errorf("outer execution context: %w", original)
	annotated = scanErrorAtRecord(outer, 4)
	if !errors.Is(annotated, outer) || !errors.Is(annotated, programExecutionFailure) {
		t.Fatalf("record annotation dropped outer error context: %v", annotated)
	}
}
