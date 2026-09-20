package bruteutils

import (
	"context"
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/oracleprobe"
)

func TestOracleBruteResultClassification(t *testing.T) {
	for _, test := range []struct {
		name                    string
		codes                   []int
		ok, finished, eliminate bool
	}{
		{"success", []int{0}, true, true, false},
		{"service_then_success", []int{12514, 0}, true, true, false},
		{"wrong_password_keeps_candidates", []int{1017}, false, false, false},
		{"locked_only_eliminates_user", []int{28000}, false, false, true},
		{"locked_in_one_service", []int{28000, 1017}, false, false, false},
		{"unknown_service", []int{12514}, false, true, false},
		{"transport_failure", []int{-1}, false, true, false},
		{"unsupported_encoding_keeps_candidates", []int{-2}, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			r := oracleBrutePass(&BruteItem{Target: "127.0.0.1", Username: "SYS", Password: "dummy"}, func(ctx context.Context, o oracleprobe.Options) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing shared timeout")
				}
				if !o.SysDBA || o.Address != "127.0.0.1:1521" {
					t.Fatalf("bad options: %#v", o)
				}
				code := test.codes[min(calls, len(test.codes)-1)]
				calls++
				if code == 0 {
					return nil
				}
				if code == -1 {
					return errors.New("transport failed")
				}
				if code == -2 {
					return oracleprobe.ErrUnsupportedCredentialEncoding
				}
				return &oracleprobe.Error{Code: code}
			})
			if r.Ok != test.ok || r.Finished != test.finished || r.UserEliminated != test.eliminate {
				t.Fatalf("unexpected result: %#v", r)
			}
		})
	}
}

func TestOracleBruteCancellationStopsServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	r := oracleBrutePass(&BruteItem{Context: ctx, Target: "127.0.0.1", Username: "probe"}, func(context.Context, oracleprobe.Options) error {
		calls++
		cancel()
		return &oracleprobe.Error{Code: 12514}
	})
	if calls != 1 || r.Ok || !r.Finished || r.UserEliminated {
		t.Fatalf("cancellation: calls=%d result=%#v", calls, r)
	}
}
