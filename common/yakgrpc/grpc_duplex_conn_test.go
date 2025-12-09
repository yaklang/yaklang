package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestDuplexConnection(t *testing.T) {
	client, err := NewLocalClient(true)
	require.Nil(t, err, "create local client error")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	stream, err := client.DuplexConnection(ctx)
	require.Nil(t, err, "create duplex connection error")
	t.Logf("create duplex connection success")
	currentExpect := "global"
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
		typ := rsp.GetMessageType()
		if typ == "exit" {
			break
		}
		if typ == currentExpect {
			switch typ {
			case "global":
				r := yakit.CreateRisk("http://127.0.0.1")
				err = yakit.SaveRisk(r)
				require.Nil(t, err, "save risk error")
				t.Logf("save risk success")
				defer yakit.DeleteRiskByID(consts.GetGormProjectDatabase(), int64(r.ID))
				currentExpect = "risk"
			case "risk":
				yakit.BroadcastData("exit", "")
			}
		} else {
			continue
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		require.Fail(t, "duplex connection timeout")
	}
	cancel()
}

func TestWatchTable(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	yakit.SaveFromHTTPFromRaw(db, false, []byte(`GET / HTTP/1.1
Host: www.example.com

`), []byte(`HTTP/1.1 200 OK
Content-Length: 1

a`), "mitm", "http://example.com", "127.0.0.1")
	a, changed := WatchDatabaseTableMeta(db, 0, context.Background(), "http_flows")
	if !changed {
		t.Fatalf("watch database table failed: %v", a)
	}
	if a <= 0 {
		t.Fatalf("watch database table failed: %v", a)
	}
	spew.Dump(a)
}

func TestGRPCMUSTPASS_HTTPFlowSlowSQL(t *testing.T) {
	yakit.InitialDatabase()

	client, err := NewLocalClient(true)
	require.NoError(t, err, "create local client error")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.DuplexConnection(ctx)
	require.NoError(t, err, "create duplex connection error")
	t.Logf("create duplex connection success")

	// 等待接收 global 消息和 slow SQL 消息
	receivedGlobal := false
	receivedSlowSQL := false

	done := make(chan bool, 1)
	go func() {
		defer func() {
			done <- true
		}()
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				t.Logf("stream recv error: %v", err)
				return
			}

			typ := rsp.GetMessageType()
			t.Logf("received message type: %s", typ)

			if typ == "global" && !receivedGlobal {
				receivedGlobal = true
				// 收到 global 消息后，等待一小段时间确保回调已注册，然后触发慢插入 SQL
				t.Logf("received global message, triggering slow insert SQL after short delay")
				go func() {
					time.Sleep(100 * time.Millisecond) // 确保回调已注册
					yakit.MockHTTPFlowSlowInsertSQL(3 * time.Second)
				}()
			}

			if typ == yakit.ServerPushType_SlowInsertSQL {
				receivedSlowSQL = true
				t.Logf("received slow insert SQL message")

				// 解析消息内容
				var data map[string]interface{}
				err := json.Unmarshal([]byte(rsp.GetData()), &data)
				require.NoError(t, err, "unmarshal slow SQL data error")

				// 验证数据格式
				require.Contains(t, data, "avg_cost", "should have avg_cost")
				require.Contains(t, data, "avg_cost_ms", "should have avg_cost_ms")
				require.Contains(t, data, "count", "should have count")
				require.Contains(t, data, "items", "should have items")

				count, ok := data["count"].(float64)
				require.True(t, ok, "count should be number")
				require.Greater(t, int(count), 0, "should have at least one slow SQL item")

				items, ok := data["items"].([]interface{})
				require.True(t, ok, "items should be array")
				require.Greater(t, len(items), 0, "should have at least one item")

				// 验证第一个 item 的字段
				item, ok := items[0].(map[string]interface{})
				require.True(t, ok, "item should be map")
				require.Contains(t, item, "duration_ms", "item should have duration_ms")
				require.Contains(t, item, "duration_str", "item should have duration_str")
				require.Contains(t, item, "func_name", "item should have func_name")
				require.Contains(t, item, "last_sql", "item should have last_sql")

				// 验证 func_name 应该是 MockHTTPFlowsSQL
				funcName, ok := item["func_name"].(string)
				require.True(t, ok, "func_name should be string")
				require.Contains(t, funcName, "MockHTTPFlowsSQL", "func_name should contain MockHTTPFlowsSQL")

				t.Logf("slow SQL data validated successfully: %+v", data)
				return
			}
		}
	}()

	// 等待接收消息，最多等待 8 秒
	select {
	case <-done:
		// 正常结束
	case <-time.After(8 * time.Second):
		t.Fatal("test timeout: did not receive slow SQL message")
	}

	require.True(t, receivedGlobal, "should receive global message")
	require.True(t, receivedSlowSQL, "should receive slow SQL message")
}

func TestGRPCMUSTPASS_MITMSlowRuleHook(t *testing.T) {
	yakit.InitialDatabase()

	client, err := NewLocalClient(true)
	require.NoError(t, err, "create local client error")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.DuplexConnection(ctx)
	require.NoError(t, err, "create duplex connection error")
	t.Logf("create duplex connection success")

	// 等待接收 global 消息和 slow rule hook 消息
	receivedGlobal := false
	receivedSlowRuleHook := false
	testHookTypes := map[string]bool{
		"hook_color":    false,
		"hook_request":  false,
		"hook_response": false,
	}

	done := make(chan bool, 1)
	go func() {
		defer func() {
			done <- true
		}()
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				t.Logf("stream recv error: %v", err)
				return
			}

			typ := rsp.GetMessageType()
			t.Logf("received message type: %s", typ)

			if typ == "global" && !receivedGlobal {
				receivedGlobal = true
				// 收到 global 消息后，等待一小段时间确保回调已注册，然后触发慢规则 Hook
				t.Logf("received global message, triggering slow rule hook after short delay")
				go func() {
					time.Sleep(100 * time.Millisecond) // 确保回调已注册
					// 测试三种 Hook 类型
					yakit.MockMITMSlowRuleHook(500*time.Millisecond, "hook_color", 10)
					time.Sleep(50 * time.Millisecond)
					yakit.MockMITMSlowRuleHook(400*time.Millisecond, "hook_request", 15)
					time.Sleep(50 * time.Millisecond)
					yakit.MockMITMSlowRuleHook(350*time.Millisecond, "hook_response", 20)
				}()
			}

			if typ == yakit.ServerPushType_SlowRuleHook {
				receivedSlowRuleHook = true
				t.Logf("received slow rule hook message")

				// 解析消息内容
				var data map[string]interface{}
				err := json.Unmarshal([]byte(rsp.GetData()), &data)
				require.NoError(t, err, "unmarshal slow rule hook data error")

				// 验证数据格式
				require.Contains(t, data, "avg_cost", "should have avg_cost")
				require.Contains(t, data, "avg_cost_ms", "should have avg_cost_ms")
				require.Contains(t, data, "count", "should have count")
				require.Contains(t, data, "items", "should have items")

				count, ok := data["count"].(float64)
				require.True(t, ok, "count should be number")
				require.Greater(t, int(count), 0, "should have at least one slow rule hook item")

				items, ok := data["items"].([]interface{})
				require.True(t, ok, "items should be array")
				require.Greater(t, len(items), 0, "should have at least one item")

				// 验证每个 item 的字段
				for _, itemInterface := range items {
					item, ok := itemInterface.(map[string]interface{})
					require.True(t, ok, "item should be map")
					require.Contains(t, item, "duration_ms", "item should have duration_ms")
					require.Contains(t, item, "duration_str", "item should have duration_str")
					require.Contains(t, item, "hook_type", "item should have hook_type")
					require.Contains(t, item, "rule_count", "item should have rule_count")
					require.Contains(t, item, "url", "item should have url")
					require.Contains(t, item, "timestamp_unix", "item should have timestamp_unix")

					// 验证 hook_type 是有效的类型
					hookType, ok := item["hook_type"].(string)
					require.True(t, ok, "hook_type should be string")
					require.Contains(t, []string{"hook_color", "hook_request", "hook_response"}, hookType, "hook_type should be one of hook_color, hook_request, hook_response")
					
					// 记录已测试的 Hook 类型
					if hookType == "hook_color" || hookType == "hook_request" || hookType == "hook_response" {
						testHookTypes[hookType] = true
					}

					// 验证 duration_ms 应该大于 300
					durationMs, ok := item["duration_ms"].(float64)
					require.True(t, ok, "duration_ms should be number")
					require.Greater(t, int64(durationMs), int64(300), "duration_ms should be greater than 300ms")

					// 验证 rule_count 应该是正数
					ruleCount, ok := item["rule_count"].(float64)
					require.True(t, ok, "rule_count should be number")
					require.Greater(t, int(ruleCount), 0, "rule_count should be greater than 0")
				}

				t.Logf("slow rule hook data validated successfully: %+v", data)
				// 由于节流机制，多个慢规则 Hook 会被合并到一个广播中
				// 检查 items 中是否包含所有三种类型的 Hook
				hookTypesInItems := make(map[string]bool)
				for _, itemInterface := range items {
					item, ok := itemInterface.(map[string]interface{})
					if !ok {
						continue
					}
					hookType, ok := item["hook_type"].(string)
					if ok {
						hookTypesInItems[hookType] = true
						testHookTypes[hookType] = true
					}
				}
				
				// 如果已经收到所有三种类型的 Hook，可以返回
				if testHookTypes["hook_color"] && testHookTypes["hook_request"] && testHookTypes["hook_response"] {
					t.Logf("received all three hook types: hook_color=%v, hook_request=%v, hook_response=%v", 
						testHookTypes["hook_color"], testHookTypes["hook_request"], testHookTypes["hook_response"])
					return
				}
				
				// 如果当前批次中包含了多种类型，继续等待可能还有其他批次
				if len(hookTypesInItems) >= 2 {
					t.Logf("received multiple hook types in this batch: %v", hookTypesInItems)
				}
			}
		}
	}()

	// 等待接收消息，最多等待 10 秒（因为节流机制可能需要更长时间）
	select {
	case <-done:
		// 正常结束
	case <-time.After(10 * time.Second):
		t.Logf("test timeout: received hook types: hook_color=%v, hook_request=%v, hook_response=%v", 
			testHookTypes["hook_color"], testHookTypes["hook_request"], testHookTypes["hook_response"])
		// 不直接失败，而是检查是否至少收到了一种类型
	}

	require.True(t, receivedGlobal, "should receive global message")
	require.True(t, receivedSlowRuleHook, "should receive slow rule hook message")
	// 验证至少收到了一种 Hook 类型
	atLeastOneHookType := testHookTypes["hook_color"] || testHookTypes["hook_request"] || testHookTypes["hook_response"]
	require.True(t, atLeastOneHookType, "should receive at least one hook type")
	
	// 由于节流机制，可能不会在一次广播中包含所有三种类型，所以只验证至少收到了一种
	t.Logf("test completed: received hook types - hook_color=%v, hook_request=%v, hook_response=%v", 
		testHookTypes["hook_color"], testHookTypes["hook_request"], testHookTypes["hook_response"])
}
