package sfreport

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sarif"
)

// TestSarifReport_StreamedDocumentIsValidJSON guards the streaming writer: the
// incremental header/run/footer writes must still produce a parseable SARIF
// document, including the edge case where no run is ever emitted.
func TestSarifReport_StreamedDocumentIsValidJSON(t *testing.T) {
	t.Run("no runs", func(t *testing.T) {
		report, err := NewSarifReport()
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, report.SetWriter(&buf))
		require.NoError(t, report.Save())

		var decoded sarif.Report
		require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded), "empty report must stay valid JSON: %s", buf.String())
		require.Empty(t, decoded.Runs)
	})

	t.Run("runs written before and after SetWriter stay ordered and valid", func(t *testing.T) {
		report, err := NewSarifReport()
		require.NoError(t, err)

		r1 := sarif.NewRunWithInformationURI("test", "https://example.com")
		r2 := sarif.NewRunWithInformationURI("test", "https://example.com")
		report.report.AddRun(r1)

		var buf bytes.Buffer
		require.NoError(t, report.SetWriter(&buf))
		report.report.AddRun(r2)
		report.streamMu.Lock()
		require.NoError(t, report.writeRunLocked(r2))
		report.streamMu.Unlock()
		require.NoError(t, report.Save())

		var decoded sarif.Report
		require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded), "streamed report must stay valid JSON: %s", buf.String())
		require.Len(t, decoded.Runs, 2)
	})

	t.Run("save is idempotent once the document is closed", func(t *testing.T) {
		report, err := NewSarifReport()
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, report.SetWriter(&buf))

		r1 := sarif.NewRunWithInformationURI("test", "https://example.com")
		report.streamMu.Lock()
		require.NoError(t, report.writeRunLocked(r1))
		report.streamMu.Unlock()

		require.NoError(t, report.Save())
		first := buf.String()
		require.NoError(t, report.Save(), "a second Save must not append a second document")
		require.Equal(t, first, buf.String())

		var decoded sarif.Report
		require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded), "output must stay valid JSON: %s", buf.String())
		require.Len(t, decoded.Runs, 1)
	})
}
