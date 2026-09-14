package csharp2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCSharpIntegerRadixSuffixAndRange(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want any
	}{
		{name: "decimal leading zero", raw: "0123", want: int64(123)},
		{name: "decimal invalid as octal", raw: "018", want: int64(18)},
		{name: "hex uppercase prefix", raw: "0X7B", want: int64(123)},
		{name: "binary uppercase prefix", raw: "0B111_1011L", want: int64(123)},
		{name: "small unsigned", raw: "1_23u", want: uint64(123)},
		{name: "unsigned long mixed suffix", raw: "1_23Lu", want: uint64(123)},
		{name: "ulong max decimal", raw: "18_446_744_073_709_551_615UL", want: ^uint64(0)},
		{name: "ulong max hex", raw: "0xFFFF_FFFF_FFFF_FFFFul", want: ^uint64(0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, parseCSharpInteger(test.raw))
		})
	}
}
