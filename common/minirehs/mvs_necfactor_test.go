package minirehs

import "testing"

func TestNecFactorRunIsNecessary(t *testing.T) {
	cases := []struct {
		name string
		expr string
		data []byte
	}{
		{
			name: "repeated group with separators",
			expr: `(?:[a-fA-F0-9]{2}:){5}[a-fA-F0-9]{2}`,
			data: []byte(`11:22:33:44:55:66`),
		},
		{
			name: "uniform repeat remains multiplicative",
			expr: `[0-9]{13}`,
			data: []byte(`1234567890123`),
		},
		{
			name: "concat internal runs do not concatenate across separator",
			expr: `[0-9]{3}:[0-9]{4}`,
			data: []byte(`123:4567`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nf := extractNecFactor(tc.expr, false, false)
			if !nf.check(tc.data) {
				t.Fatalf("necessary factor rejected a matching input: expr=%q data=%q factor=%+v",
					tc.expr, tc.data, nf)
			}
		})
	}
}

func TestNecFactorSeparatedRepeatDoesNotInventContinuousRun(t *testing.T) {
	nf := extractNecFactor(`(?:[a-fA-F0-9]{2}:){5}[a-fA-F0-9]{2}`, false, false)
	if nf.minRunLen > 2 {
		t.Fatalf("separated hex pairs produced unsafe continuous run length %d", nf.minRunLen)
	}
}
