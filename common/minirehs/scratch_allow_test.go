package minirehs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestrictScratchSkipsDisallowedPattern(t *testing.T) {
	db, err := Compile([]Pattern{
		{ID: 0, Expr: "alpha"},
		{ID: 1, Expr: "beta"},
	}, WithBackend(BackendEngine))
	require.NoError(t, err)
	defer db.Close()
	scratch, err := db.NewScratch()
	require.NoError(t, err)
	defer scratch.Close()

	var got []PatternID
	scan := func() {
		got = nil
		require.NoError(t, db.Scan([]byte("alpha beta"), scratch, func(m Match) bool {
			got = append(got, m.ID)
			return true
		}))
	}
	RestrictScratch(scratch, []bool{true, false})
	scan()
	require.Equal(t, []PatternID{0}, got)

	RestrictScratch(scratch, nil)
	scan()
	require.ElementsMatch(t, []PatternID{0, 1}, got)
}
