package aicommon

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUICanonicalFieldStreamIgnoresNestedNames(t *testing.T) {
	const ui = `{"id":"block","props":{"ui":{"id":"not-another-block"}}}`
	for _, input := range []string{
		`{"@action":"render_ui","ui":` + ui + `}`,
		`{"next_action":{"@action":"render_ui","ui":` + ui + `}}`,
	} {
		output := make(chan string, 8)
		maker := NewActionMaker("render_ui", WithActionCanonicalStreamFields("ui"),
			WithActionFieldStreamHandler([]string{"ui"}, func(_ string, reader io.Reader) {
				raw, _ := io.ReadAll(reader)
				output <- string(raw)
			}))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		action := maker.ReadFromReader(ctx, strings.NewReader(input))
		require.NotNil(t, action)
		select {
		case got := <-output:
			require.JSONEq(t, ui, got)
		case <-ctx.Done():
			t.Fatal("UI field did not finish")
		}
		select {
		case unexpected := <-output:
			t.Fatalf("nested field opened an extra stream: %s", unexpected)
		default:
		}
		cancel()
	}
}
