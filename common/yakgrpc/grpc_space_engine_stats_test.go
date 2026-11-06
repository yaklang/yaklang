package yakgrpc

import (
	"context"
	"maps"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SpaceEngineStatusCards(t *testing.T) {
	records := []*base.NetSpaceEngineResult{
		{Addr: "192.0.2.1:443", Host: "first.example", HtmlTitle: "first"},
		{Addr: "192.0.2.1:443", Host: "second.example", HtmlTitle: "second"},
		{Addr: "192.0.2.1:80"},
		{Addr: "[2001:db8::1]:443"},
		{Addr: "[2001:db8::1]:443", HtmlTitle: "IPv6 update"},
		{Addr: "first.example:443"},
		{Addr: "second.example:443"},
	}
	type testCase struct {
		name, engine string
		scan         bool
		records      []*base.NetSpaceEngineResult
		wantUnique   int
	}
	var cases []testCase
	for _, name := range []string{"fofa", "shodan", "quake", "hunter", "zoomeye", "zone"} {
		cases = append(cases, testCase{name: name, engine: name, records: records, wantUnique: 5})
	}
	cases = append(cases,
		testCase{name: "verification updates", engine: "hunter", scan: true, records: records, wantUnique: 5},
		testCase{name: "empty results", engine: "fofa"},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var mu sync.Mutex
			cards := make(map[string]string)
			rows := make(map[string]string)
			var writes, saved int
			client := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
				if !result.IsMessage {
					return nil
				}
				content := gjson.GetBytes(result.Message, "content")
				data := gjson.Parse(content.Get("data").String())
				mu.Lock()
				defer mu.Unlock()
				switch content.Get("level").String() {
				case "feature-status-card-data":
					cards[data.Get("id").String()] = data.Get("data").String()
				case "feature-table-data":
					rows[data.Get("data.uuid").String()] = data.Get("data.网站标题").String()
					writes++
				}
				return nil
			})
			queryResults := make(chan *base.NetSpaceEngineResult, len(tc.records))
			for _, record := range tc.records {
				queryResults <- record
			}
			close(queryResults)
			spaceEngineLib := maps.Clone(yaklib.SpaceEngineExports)
			spaceEngineLib["Query"] = func(string, ...any) (chan *base.NetSpaceEngineResult, error) {
				return queryResults, nil
			}
			engine := yak.NewYakitVirtualClientScriptEngine(client)
			engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
				engine.SetVars(map[string]any{
					"FILTER": "test", "ENGINE_TYPE": tc.engine, "SCAN_VERIFY": tc.scan,
					"spacengine": spaceEngineLib,
					// Disable periodic reports so final counts must be flushed before returning.
					"sleep": func(seconds float64) {
						if seconds == 1 {
							<-ctx.Done()
						}
					},
					"servicescan": map[string]any{
						"probeTimeout": func(int) any { return nil },
						"ScanOne": func(host string, port int, _ ...any) (*fp.MatchResult, error) {
							return &fp.MatchResult{Target: host, Port: port}, nil
						},
					},
					"db": map[string]any{"SavePortFromResult": func(*fp.MatchResult) {
						mu.Lock()
						saved++
						mu.Unlock()
					}},
				})
				return nil
			})
			require.NoError(t, engine.ExecuteWithContext(ctx, spaceEngineExecCode))
			cancel()
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, strconv.Itoa(len(tc.records)), cards["已获取记录"])
			require.Equal(t, strconv.Itoa(tc.wantUnique), cards["唯一端点"])
			require.NotContains(t, cards, "获取数据")
			require.Len(t, rows, tc.wantUnique, "the unique count must match the table's UUID count")
			if tc.scan {
				require.Equal(t, 2*len(tc.records), writes, "verification must update the same table rows")
				require.Equal(t, len(tc.records), saved)
				require.Equal(t, strconv.Itoa(len(tc.records)), cards["验证扫描"])
			} else {
				require.Equal(t, len(tc.records), writes, "all fetched records must still reach the table")
				if len(tc.records) > 0 {
					require.Equal(t, "second", rows["192.0.2.1:443"])
				}
			}
		})
	}
}
