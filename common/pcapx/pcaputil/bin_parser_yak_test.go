package pcaputil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	yaklang "github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestBinParserYakAPI(t *testing.T) {
	wire := string(binMQTTConnect) + string(binMQTTPublish(32))
	name := filepath.Join(t.TempDir(), "messages.pcap")
	require.NoError(t, os.WriteFile(name, binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 1, data: wire}}, 1883, false, false), 0600))
	for _, workers := range []int{1, 2, 4} {
		engine := yaklang.New()
		var recording bytes.Buffer
		engine.SetVars(map[string]any{"pcapx": Exports, "captureContext": context.Background(), "recording": &recording, "capture": name, "workers": workers, "check": func(ok bool) { require.True(t, ok) }, "rowCount": func(rows []*BinParserEvent) int { return len(rows) }})
		err := engine.SafeEval(context.Background(), `
view = pcapx.NewBinParserInspector(32, 65536)~
pcapx.ReplayPcapFile(capture,
    pcapx.pcap_context(captureContext),
    pcapx.pcap_tcpReassemblyWorkers(workers),
    pcapx.pcap_binParser(func(event) { view.OnEvent(event) }),
    pcapx.pcap_binParserDeferred(true),
    pcapx.pcap_captureWriter(recording),
)~
rows = view.Rows("mqtt", 0)
check(rowCount(rows) == 2)
detail = view.Details(rows[1].ID)~
check(detail.Status == "decoded")
`)
		require.NoError(t, err)
		events, _, err := binReplay(t, recording.Bytes(), workers)
		require.NoError(t, err)
		require.Len(t, events, 2)
	}
}
