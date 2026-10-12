package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type exactProtocolNumber string

// Answers must retain their original decimal tokens. A float64 loader loses
// adjacent integer values above 2^53 before any comparison can recover them.
func decodeExactProtocolAnswer(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("oracle must contain exactly one JSON value: %v", err)
	}
	return nil
}
func canonicalExactProtocolFields(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			n, err := canonicalExactProtocolFields(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = n
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			n, err := canonicalExactProtocolFields(v)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	case json.Number:
		n, ok := new(big.Rat).SetString(string(x))
		if !ok {
			return nil, fmt.Errorf("invalid JSON number %q", x)
		}
		return exactProtocolNumber(n.RatString()), nil
	default:
		return v, nil
	}
}
func rejectLossyProtocolOracle(want, got any) error {
	switch x := want.(type) {
	case map[string]any:
		g, _ := got.(map[string]any)
		for k, v := range x {
			if err := rejectLossyProtocolOracle(v, g[k]); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
	case []any:
		g := reflect.ValueOf(got)
		for i, v := range x {
			var actual any
			if g.IsValid() && g.Kind() == reflect.Slice && i < g.Len() {
				actual = g.Index(i).Interface()
			}
			if err := rejectLossyProtocolOracle(v, actual); err != nil {
				return err
			}
		}
	case float64:
		// IEEE floating wire fields are intentionally floating values. The unsafe
		// case is an integer/decimal-token output checked with a rounded float oracle.
		switch got.(type) {
		case float32, float64:
			return nil
		}
		if math.Abs(x) >= 1<<53 && math.Trunc(x) == x {
			return fmt.Errorf("large integral oracle was loaded through float64; load original JSON with UseNumber")
		}
	}
	return nil
}

func exactProtocolFieldsEqual(want, got map[string]any) (bool, error) {
	return exactProtocolFieldsEqualMode(want, got, true)
}
func exactProtocolFieldsEqualMode(want, got map[string]any, fast bool) (bool, error) {
	if err := rejectLossyProtocolOracle(want, got); err != nil {
		return false, err
	}
	wireWant, err := json.Marshal(want)
	if err != nil {
		return false, err
	}
	wireGot, err := json.Marshal(got)
	if err != nil {
		return false, err
	}
	// Equal canonical JSON bytes already prove every key, value, array order
	// and numeric token equal. Keep the lossy-oracle guard above this path.
	// Different encodings (1e3/1000, decimals, signed zero) still use the exact
	// rational comparison below, without a second marshal of either tree.
	if fast && bytes.Equal(wireWant, wireGot) {
		return true, nil
	}
	normalize := func(b []byte) (any, error) {
		var n any
		if err := decodeExactProtocolAnswer(b, &n); err != nil {
			return nil, err
		}
		return canonicalExactProtocolFields(n)
	}
	w, err := normalize(wireWant)
	if err != nil {
		return false, err
	}
	g, err := normalize(wireGot)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(w, g), nil
}
func requireExactProtocolFields(t *testing.T, want, got map[string]any) {
	t.Helper()
	equal, err := exactProtocolFieldsEqual(want, got)
	require.NoError(t, err)
	require.True(t, equal, "exact complete protocol fields differ: expected %#v, actual %#v", want, got)
}
func TestProtocolExactFieldIntegerOracle(t *testing.T) {
	cases := []struct {
		name, oracle string
		actual       any
		equal        bool
	}{
		{"above-double-boundary", "9007199254740993", uint64(9007199254740993), true},
		{"adjacent-double-boundary", "9007199254740993", uint64(9007199254740992), false},
		{"max-uint64", "18446744073709551615", ^uint64(0), true},
		{"adjacent-max-uint64", "18446744073709551615", ^uint64(0) - 1, false},
		{"numeric-exponent", "1e3", 1000, true},
		{"large-floating-wire-field", "1e30", float64(1e30), true},
		{"decimal-value", "0.50", json.Number("0.5"), true},
		{"number-versus-string", "1000", "1000", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var want map[string]any
			require.NoError(t, decodeExactProtocolAnswer([]byte(`{"nested":[{"value":`+c.oracle+`}]}`), &want))
			got := map[string]any{"nested": []any{map[string]any{"value": c.actual}}}
			equal, err := exactProtocolFieldsEqual(want, got)
			require.NoError(t, err)
			require.Equal(t, c.equal, equal)
			oldEqual, oldErr := exactProtocolFieldsEqualMode(want, got, false)
			require.NoError(t, oldErr)
			require.Equal(t, oldEqual, equal, "fast path must retain exact rational semantics")
		})
	}
	equal, err := exactProtocolFieldsEqual(map[string]any{"counter": float64(9007199254740992)}, map[string]any{"counter": uint64(9007199254740993)})
	require.False(t, equal)
	require.ErrorContains(t, err, "float64")
	t.Run("canonical-tree-parity", func(t *testing.T) {
		values := []any{
			nil, true, false, "1000", "\xff", []byte{0, 255}, []byte{}, []byte(nil),
			[]any{}, []any(nil), []uint32{0, 1, ^uint32(0)},
			map[string]any{"a": json.Number("1e3"), "b": []any{json.Number("0.50"), "x"}},
			json.Number("-0"), json.Number("1e3"), json.Number("0.50"),
			int64(-1), uint64(9007199254740993), ^uint64(0), float32(0.1), float64(1e30),
		}
		for i, want := range values {
			for j, got := range values {
				w, g := map[string]any{"field": want}, map[string]any{"field": got}
				oldEqual, oldErr := exactProtocolFieldsEqualMode(w, g, false)
				newEqual, newErr := exactProtocolFieldsEqual(w, g)
				require.Equal(t, oldEqual, newEqual, "pair%d/%d", i, j)
				if oldErr != nil {
					require.EqualError(t, newErr, oldErr.Error())
				} else {
					require.NoError(t, newErr)
				}
			}
		}
	})
}
