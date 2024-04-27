package yakgrpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
		result := gjson.ParseBytes(rsp.Data)
		typ := result.Get("type").String()
		if typ == "global" {
			r := yakit.CreateRisk("http://127.0.0.1")
			err = yakit.SaveRisk(r)
			require.Nil(t, err, "save risk error")
			t.Logf("save risk success")
			defer yakit.DeleteRiskByID(consts.GetGormProjectDatabase(), int64(r.ID))
			continue
		}
		require.Equal(t, "risk", typ, "type not match")
		break
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
