package permutil

import (
	"strings"
	"testing"
)

func TestBuildWindowsSudoBatchLines(t *testing.T) {
	lines := buildWindowsSudoBatchLines(
		"yak version",
		"NUL",
		"NUL",
		"",
		false,
		true,
		`C:\yak`,
		map[string]string{"FOO": "bar"},
	)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, `cd "C:\\yak"`) {
		t.Fatalf("expected workdir change, got:\n%s", joined)
	}
	if !strings.Contains(joined, `set FOO=bar`) {
		t.Fatalf("expected environment export, got:\n%s", joined)
	}
	if !strings.Contains(joined, `call :sub > NUL 2> NUL`) {
		t.Fatalf("expected NUL redirection, got:\n%s", joined)
	}
	if strings.Contains(joined, `echo %yak_exit_code% >`) {
		t.Fatalf("did not expect exit code file write, got:\n%s", joined)
	}
	if !strings.Contains(joined, `del /f /q "%~f0" >nul 2>nul`) {
		t.Fatalf("expected self-delete command, got:\n%s", joined)
	}
}
