package yak

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAIToolRuntimeBindingsPreserveExports(t *testing.T) {
	for _, tc := range []struct {
		module, member string
		bind           func(*antlr4yak.Engine, context.Context, *aitool.ToolRuntimeConfig)
	}{
		{"db", "SaveHTTPFlowFromRawWithOption", bindDBHistoryToEngine},
		{"aimemory", "memoryLimit", bindMemorySearchToEngine},
		{"aihistory", "limit", bindAIHistoryToEngine},
	} {
		t.Run(tc.module, func(t *testing.T) {
			engine := yaklang.New()
			called := false
			engine.SetVars(map[string]any{tc.module: map[string]any{tc.member: func() { called = true }}})
			tc.bind(engine, context.Background(), &aitool.ToolRuntimeConfig{})
			require.NoError(t, engine.SafeEval(context.Background(), tc.module+"."+tc.member+"()"))
			require.True(t, called, "runtime binding replaced an unrelated engine export")
		})
	}
}

func TestAIToolRuntimeClientAndRiskSinkIsolation(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	previous := consts.CaptureProjectDatabaseBinding()
	consts.BindProjectDatabaseWithReader(db, nil, "")
	t.Cleanup(func() { consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path) })
	previousClient := yaklib.GetYakitClientInstance()
	defaultClient := yaklib.NewVirtualYakitClient(func(*ypb.ExecResult) error { return nil })
	yaklib.InitYakit(defaultClient)
	t.Cleanup(func() { yaklib.InitYakit(previousClient) })
	code := `
marker = cli.String("marker")
risk.NewRisk("127.0.0.1", risk.title(marker), risk.type("xss"))
r = risk.CreateRisk("127.0.0.1", risk.title(marker), risk.type("sqli"))
risk.Save(r)~
yakit.Info("marker:%s namespace:%s session:%s", marker, aimemory.CurrentNamespace(), aihistory.CurrentSession())
yakit.Debug("debug:%s", marker)
yakit.EnableText(marker)
yakit.TextTabData(marker, marker)
`
	tool := YakTool2AITool([]*schema.AIYakTool{{Name: "runtime-isolation", Content: code, Params: `{"type":"object","properties":{"marker":{"type":"string"}}}`}})[0]
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			marker := fmt.Sprintf("isolated-%d", i)
			var risks []*schema.Risk
			var outputs []*ypb.ExecResult
			runtime := &aitool.ToolRuntimeConfig{
				RuntimeID: marker, MemoryNamespace: marker, PersistentSessionID: marker,
				RiskSaveHandler: func(_ context.Context, risk *schema.Risk) error {
					copy := *risk
					risks = append(risks, &copy)
					return nil
				},
				FeedBacker: func(output *ypb.ExecResult) error {
					outputs = append(outputs, output)
					return nil
				},
			}
			var stdout string
			var invokeErr error
			if i%2 == 0 {
				config := aitool.NewToolInvokeConfig()
				aitool.WithRuntimeConfig(runtime)(config)
				result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"marker": marker}, config)
				invokeErr = err
				if result != nil {
					stdout = result.Stdout
				}
			} else {
				var out, stderr bytes.Buffer
				_, invokeErr = executeNativeYakPlugin(context.Background(), &schema.YakScript{ScriptName: marker, Content: code}, aitool.InvokeParams{"marker": marker}, runtime, &out, &stderr)
				stdout = out.String()
			}
			if invokeErr != nil {
				t.Errorf("%s: %v", marker, invokeErr)
				return
			}
			if len(risks) != 2 {
				t.Errorf("%s: risk sink received %d records, want 2", marker, len(risks))
				return
			}
			for _, risk := range risks {
				if risk.RuntimeId != marker || risk.Title != marker {
					t.Errorf("%s: cross-invocation risk: %#v", marker, risk)
				}
			}
			if !strings.Contains(stdout, "marker:"+marker+" namespace:"+marker+" session:"+marker) {
				t.Errorf("%s: incorrect output: %s", marker, stdout)
			}
			for _, output := range outputs {
				if output.RuntimeID != marker {
					t.Errorf("%s: cross-invocation output: %s", marker, output.RuntimeID)
				}
			}
			if len(outputs) != 6 {
				t.Errorf("%s: received %d output events, want 6", marker, len(outputs))
			}
		}(i)
	}
	workers.Wait()
	require.Same(t, defaultClient, yaklib.GetYakitClientInstance(), "AI invocation changed the process default client")
	var count int
	require.NoError(t, db.Model(&schema.Risk{}).Count(&count).Error)
	require.Zero(t, count, "runtime sink risks leaked into the process database")
}

func TestAIToolRuntimeRiskDatabaseIsolation(t *testing.T) {
	globalDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = globalDB.Close() })
	require.NoError(t, globalDB.AutoMigrate(&schema.Risk{}).Error)
	previous := consts.CaptureProjectDatabaseBinding()
	consts.BindProjectDatabaseWithReader(globalDB, nil, "")
	t.Cleanup(func() { consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path) })
	code := `
risk.NewRisk("127.0.0.1", risk.title("runtime-bound"), risk.type("xss"))
r = risk.CreateRisk("127.0.0.1", risk.title("runtime-bound"), risk.type("sqli"))
risk.Save(r)~
page = risk.QueryRiskInDatabase(nil)~
yakit.Info("bound-risks:%d", page.Total)
`
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			db, err := utils.CreateTempTestDatabaseInMemory()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
			runtime := &aitool.ToolRuntimeConfig{RuntimeID: t.Name(), ProjectDatabase: db}
			var stdout string
			if native {
				var out, stderr bytes.Buffer
				result, err := executeNativeYakPlugin(context.Background(), &schema.YakScript{ScriptName: "bound-risk", Content: code}, nil, runtime, &out, &stderr)
				require.NoError(t, err)
				require.Contains(t, result, "Vulnerabilities Found: 2")
				stdout = out.String()
			} else {
				tool := YakTool2AITool([]*schema.AIYakTool{{Name: "bound-risk", Content: code, Params: `{"type":"object","properties":{}}`}})[0]
				config := aitool.NewToolInvokeConfig()
				aitool.WithRuntimeConfig(runtime)(config)
				result, err := tool.ExecuteToolWithCapture(context.Background(), nil, config)
				require.NoError(t, err)
				stdout = result.Stdout
			}
			require.Contains(t, stdout, "bound-risks:2")
			records, err := yakit.GetRisksByRuntimeId(db, runtime.RuntimeID)
			require.NoError(t, err)
			require.Len(t, records, 2)
		})
	}
	var count int
	require.NoError(t, globalDB.Model(&schema.Risk{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAIToolRiskMissingClientAndRecord(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	previous := consts.CaptureProjectDatabaseBinding()
	consts.BindProjectDatabaseWithReader(db, nil, "")
	t.Cleanup(func() { consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path) })
	save := yaklib.RiskExports["Save"].(func(*schema.Risk) error)
	require.ErrorContains(t, save(nil), "risk is required")
	require.ErrorContains(t, yakit.SaveRisk(nil), "risk is required")
	require.ErrorContains(t, yakit.SaveRiskWithDatabase(nil, &schema.Risk{}), "no database connection")
	record := yakit.CreateRisk("127.0.0.1", yakit.WithRiskParam_Title("no-client"))
	require.NoError(t, save(record), "package-initialized risk exports must tolerate a missing output client")
	yaklib.YakitNewRiskBuilder(nil)("127.0.0.1", yakit.WithRiskParam_Title("new-no-client"), yakit.WithRiskParam_RiskType("xss"))
	require.NotPanics(t, func() { yaklib.GetExtYakitLibByClient(nil)["Info"].(func(string, ...any))("no-client-output") })
	var count int
	require.NoError(t, db.Model(&schema.Risk{}).Count(&count).Error)
	require.Equal(t, 2, count)
	previousClient := yaklib.GetYakitClientInstance()
	yaklib.InitYakit(nil)
	t.Cleanup(func() { yaklib.InitYakit(previousClient) })
	require.NotPanics(t, func() {
		yaklib.YakitExports["EnableText"].(func(string))("test")
		yaklib.YakitExports["EnableTable"].(func(string, []string))("test", nil)
		yaklib.YakitExports["EnableWebsiteTrees"].(func(string))("127.0.0.1")
		yaklib.YakitExports["EnableDotGraphTab"].(func(string))("test")
		yaklib.YakitExports["StatusCard"].(func(string, any, ...string))("test", 1)
	})
}

func TestAIToolPluginRiskRuntimeBinding(t *testing.T) {
	globalDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = globalDB.Close() })
	require.NoError(t, globalDB.AutoMigrate(&schema.Risk{}).Error)
	previous := consts.CaptureProjectDatabaseBinding()
	consts.BindProjectDatabaseWithReader(globalDB, nil, "")
	t.Cleanup(func() { consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	t.Cleanup(server.Close)
	for _, pluginType := range []string{"mitm", "port-scan"} {
		for _, sink := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sink=%v", pluginType, sink), func(t *testing.T) {
				db, err := utils.CreateTempTestDatabaseInMemory()
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })
				require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
				var risks []*schema.Risk
				var riskOutputs []*ypb.ExecResult
				runtime := &aitool.ToolRuntimeConfig{RuntimeID: t.Name(), ProjectDatabase: db, MemoryNamespace: "plugin-memory", PersistentSessionID: "plugin-session",
					FeedBacker: func(output *ypb.ExecResult) error {
						if strings.Contains(string(output.GetMessage()), "json-risk") {
							riskOutputs = append(riskOutputs, output)
						}
						return nil
					},
				}
				if sink {
					runtime.RiskSaveHandler = func(_ context.Context, risk *schema.Risk) error {
						copy := *risk
						risks = append(risks, &copy)
						return nil
					}
				}
				body := `
risk.NewRisk("127.0.0.1", risk.title("plugin-risk"), risk.type("xss"))
r = risk.CreateRisk("127.0.0.1", risk.title("plugin-risk"), risk.type("sqli"))
risk.Save(r)~
page = risk.QueryRiskInDatabase(nil)~
yakit.Info("plugin-risks:%d namespace:%s session:%s", page.Total, aimemory.CurrentNamespace(), aihistory.CurrentSession())
`
				var stdout, stderr bytes.Buffer
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				var result any
				if pluginType == "mitm" {
					script := &schema.YakScript{ScriptName: t.Name(), Type: pluginType, Content: "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {" + body + "}"}
					result, err = executeMitmPlugins(ctx, []*schema.YakScript{script}, aitool.InvokeParams{"url": server.URL}, runtime, &stdout, &stderr)
				} else {
					script := &schema.YakScript{ScriptName: t.Name(), Type: pluginType, Content: "handle = func(result) {" + body + "}"}
					result, err = executePortScanPlugins(ctx, []*schema.YakScript{script}, aitool.InvokeParams{"target": "127.0.0.1", "port": "80"}, runtime, &stdout, &stderr)
				}
				require.NoError(t, err)
				records, err := yakit.GetRisksByRuntimeId(db, runtime.RuntimeID)
				require.NoError(t, err)
				if sink {
					require.Len(t, risks, 2)
					require.Empty(t, records, "external sink must take priority over the configured database")
					require.Contains(t, stdout.String(), "plugin-risks:0 namespace:plugin-memory session:plugin-session")
					require.NotContains(t, result, "no vulnerabilities found")
					require.Contains(t, result, "runtime risk sink")
				} else {
					require.Len(t, records, 2)
					require.Contains(t, stdout.String(), "plugin-risks:2 namespace:plugin-memory session:plugin-session")
					require.Contains(t, result, "Vulnerabilities Found: 2")
				}
				require.Len(t, riskOutputs, 2)
				for _, output := range riskOutputs {
					require.Equal(t, runtime.RuntimeID, output.RuntimeID)
				}
			})
		}
	}
	var count int
	require.NoError(t, globalDB.Model(&schema.Risk{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAIToolPluginRiskSinkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	t.Cleanup(server.Close)
	for _, pluginType := range []string{"mitm", "port-scan"} {
		for _, method := range []string{"Save", "NewRisk"} {
			t.Run(pluginType+"/"+method, func(t *testing.T) {
				code := `r = risk.CreateRisk("127.0.0.1"); risk.Save(r)~`
				if method == "NewRisk" {
					code = `risk.NewRisk("127.0.0.1")`
				}
				runtime := &aitool.ToolRuntimeConfig{RuntimeID: t.Name(), RiskSaveHandler: func(context.Context, *schema.Risk) error { return errors.New("platform unavailable") }}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				var stdout, stderr bytes.Buffer
				var result any
				var err error
				if pluginType == "mitm" {
					script := &schema.YakScript{ScriptName: t.Name(), Type: pluginType, Content: "mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {" + code + "}"}
					result, err = executeMitmPlugins(ctx, []*schema.YakScript{script}, aitool.InvokeParams{"url": server.URL}, runtime, &stdout, &stderr)
				} else {
					script := &schema.YakScript{ScriptName: t.Name(), Type: pluginType, Content: "handle = func(result) {" + code + "}"}
					result, err = executePortScanPlugins(ctx, []*schema.YakScript{script}, aitool.InvokeParams{"target": "127.0.0.1", "port": "80"}, runtime, &stdout, &stderr)
				}
				require.ErrorContains(t, err, "platform unavailable")
				require.Nil(t, result, "a rejected risk must not be reported as a successful scan")
			})
		}
	}
}
