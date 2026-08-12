package script

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	s2tests "github.com/yaklang/yaklang/common/yak/ssa2llvm/tests"
)

// TestPocRequest_Effect compiles poc_request.yak via the real ssa2llvm CLI,
// points YAK_TEST_URL at a local HTTP server, and verifies the compiled binary
// fetches and prints the server's response body.
func TestPocRequest_Effect(t *testing.T) {
	want := uuid.NewString()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, want)
	}))
	t.Cleanup(srv.Close)

	output := s2tests.RunYakScriptFileWithCLI(t, "poc_request.yak", map[string]string{
		"YAK_TEST_URL": srv.URL,
	})

	if got := strings.TrimSpace(output); got != want {
		t.Fatalf("unexpected output: %q (want %s)", output, want)
	}
}
