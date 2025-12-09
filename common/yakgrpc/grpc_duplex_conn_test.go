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
