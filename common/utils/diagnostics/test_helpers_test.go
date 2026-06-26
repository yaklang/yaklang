package diagnostics

import "testing"

func withDiagnosticsLevel(t *testing.T, lvl Level) {
	t.Helper()
	orig := GetLevel()
	SetLevel(lvl)
	t.Cleanup(func() { SetLevel(orig) })
}
