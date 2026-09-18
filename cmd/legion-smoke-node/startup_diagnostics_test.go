package main

import (
	"bytes"
	"errors"
	"log"
	"regexp"
	"strings"
	"testing"
)

func TestStartupStageDiagnostics(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	startStartupStage(false, "host_stage")(nil)
	if output.Len() != 0 {
		t.Fatal("disabled stage emitted diagnostics")
	}

	finish := startStartupStage(true, "capabilities_sync")
	if !strings.Contains(output.String(), "stage=capabilities_sync status=started") {
		t.Fatal("missing start marker before initialization completes")
	}
	finish(nil)
	if !regexp.MustCompile(`stage=capabilities_sync status=completed elapsed_ms=[0-9]+\.[0-9]{3}`).MatchString(output.String()) {
		t.Fatalf("missing successful stage timing: %s", output.String())
	}

	output.Reset()
	startStartupStage(true, "sessionmgr_register")(errors.New("credential=private-diagnostic-sentinel"))
	if !strings.Contains(output.String(), "stage=sessionmgr_register status=failed elapsed_ms=") {
		t.Fatal("failed initialization was not marked failed")
	}
	if strings.Contains(output.String(), "private-diagnostic-sentinel") {
		t.Fatal("diagnostics leaked raw error details")
	}
}
