package sca

import (
	"bytes"
	"context"
	"log"
	"testing"
	"testing/fstest"
)

func TestMissingMetadataUsesDiagnosticsNotGlobalLogger(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	input := fstest.MapFS{"pom.xml": {Data: []byte(`<project><groupId>app</groupId><artifactId>app</artifactId><version>1</version><parent><groupId>missing</groupId><artifactId>parent</artifactId><version>1</version></parent></project>`)}}
	r, err := ScanReport(context.Background(), input)
	if r == nil || err == nil || r.Complete {
		t.Fatalf("missing metadata silently succeeded: %+v %v", r, err)
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "evidence_insufficient" && d.Incomplete {
			found = true
		}
	}
	if !found {
		t.Fatal("missing structured evidence diagnostic")
	}
	if output.Len() != 0 {
		t.Fatalf("core wrote process-global log: %q", output.String())
	}
}
