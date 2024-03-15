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
