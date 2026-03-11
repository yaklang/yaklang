package ssa

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeStableNamePart(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "unnamed"},
		{name: "all_symbols", raw: "___", want: "unnamed"},
		{name: "plain_ascii", raw: "hello", want: "hello"},
		{name: "dash_to_underscore", raw: "hello-world", want: "hello_world"},
		{name: "trim_wrapped_symbols", raw: "$$$name###", want: "name"},
		{name: "digits_and_letters", raw: "123abc", want: "123abc"},
		{name: "unicode_to_underscore", raw: "prefix 中文 suffix", want: "prefix____suffix"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, SanitizeStableNamePart(tc.raw))
		})
	}
}

func TestNextStableName(t *testing.T) {
	var seq int
	require.Equal(t, "tmp_1", NextStableName("", &seq, "tmp"))
	require.Equal(t, "tmp_2", NextStableName("", &seq, "tmp"))
	require.Equal(t, "logical_name_3", NextStableName("logical-name", &seq, "tmp"))
}

func TestNextStableNameDeterministicAndNoUUID(t *testing.T) {
	var seqA int
	var seqB int

	nameA1 := NextStableName("call_unpack", &seqA, "tmp")
	nameA2 := NextStableName("call_unpack", &seqA, "tmp")
	nameB1 := NextStableName("call_unpack", &seqB, "tmp")

	require.NotEqual(t, nameA1, nameA2)
	require.Equal(t, nameA1, nameB1)

	uuidPattern := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	require.False(t, uuidPattern.MatchString(nameA1))
	require.False(t, uuidPattern.MatchString(nameA2))
}

func TestNextStableNameBoundaryCases(t *testing.T) {
	t.Run("fallback defaults to tmp when empty", func(t *testing.T) {
		var seq int
		require.Equal(t, "tmp_1", NextStableName("", &seq, ""))
	})

	t.Run("nil sequence keeps deterministic suffix 1", func(t *testing.T) {
		require.Equal(t, "name_1", NextStableName("name", nil, "tmp"))
		require.Equal(t, "name_1", NextStableName("name", nil, "tmp"))
	})

	t.Run("symbol only prefix becomes unnamed", func(t *testing.T) {
		var seq int
		require.Equal(t, "unnamed_1", NextStableName("$$$", &seq, "tmp"))
	})
}
