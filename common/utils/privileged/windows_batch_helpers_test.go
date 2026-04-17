package privileged

import (
	"strings"
	"testing"
)

func TestBuildWindowsUACBatchLines(t *testing.T) {
	t.Run("sync command writes exit code and keeps batch", func(t *testing.T) {
		lines := buildWindowsUACBatchLines(
			"yak version",
			`"C:\temp\stdout.txt"`,
			`"C:\temp\stderr.txt"`,
			`C:\temp\exitcode.txt`,
			true,
			false,
		)

		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, `call :sub > "C:\temp\stdout.txt" 2> "C:\temp\stderr.txt"`) {
			t.Fatalf("expected stdout/stderr redirection, got:\n%s", joined)
		}
		if !strings.Contains(joined, `echo %yak_exit_code% > "C:\\temp\\exitcode.txt"`) {
			t.Fatalf("expected exit code redirection, got:\n%s", joined)
		}
		if strings.Contains(joined, `del /f /q "%~f0" >nul 2>nul`) {
			t.Fatalf("did not expect self-delete for sync command, got:\n%s", joined)
		}
	})

	t.Run("async command discards output and deletes batch", func(t *testing.T) {
		lines := buildWindowsUACBatchLines("yak tunnel", "NUL", "NUL", "", false, true)

		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, `call :sub > NUL 2> NUL`) {
			t.Fatalf("expected NUL redirection, got:\n%s", joined)
		}
		if strings.Contains(joined, `echo %yak_exit_code% >`) {
			t.Fatalf("did not expect exit code file write, got:\n%s", joined)
		}
		if !strings.Contains(joined, `del /f /q "%~f0" >nul 2>nul`) {
			t.Fatalf("expected self-delete for async command, got:\n%s", joined)
		}
	})
}
