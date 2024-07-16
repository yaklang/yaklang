package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_Brute(t *testing.T) {
	redisPasswd := "123456"
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	_ = cancel
	host, port := tools.DebugMockRedis(ctx, true, redisPasswd)
	target := utils.HostPort(host, port)

	host, port = tools.DebugMockRedis(ctx, false)
	unAuthTarget := utils.HostPort(host, port)

	client, err := NewLocalClient()
	stream, err := client.StartBrute(ctx, &ypb.StartBruteParams{
		Type:                       "redis",
		Targets:                    target + "\n" + unAuthTarget,
		Usernames:                  []string{},
		Passwords:                  []string{"123456"},
		ReplaceDefaultPasswordDict: true,
		ReplaceDefaultUsernameDict: true,
		OkToStop:                   true,
		Concurrent:                 50,
		TargetTaskConcurrent:       1,
		DelayMax:                   5,
		DelayMin:                   1,
	})
	require.NoError(t, err)

	var runtimeID string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.RuntimeID != "" {
			runtimeID = rsp.RuntimeID
		}
	}

	risks, err := yakit.GetRisksByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
	require.NoError(t, err)
	require.Len(t, risks, 2)

	weakPasswdOk := false
	unAuthOk := false
	for _, r := range risks {
		if strings.Contains(r.TitleVerbose, "未授权访问") {
			weakPasswdOk = true
		}
		if strings.Contains(r.TitleVerbose, "弱口令") {
			unAuthOk = true
		}
	}

	if !weakPasswdOk {
		t.Fatal("brute weak password failed")
	}
	if !unAuthOk {
		t.Fatal("brute unAuth failed")
	}
}

func TestGRPCMUSTPASS_GetBruteType(t *testing.T) {
	BuildInBruteTypeTree := GetBuildinAvailableBruteTypeTree([]struct {
		Name string
		Data string
	}{
		{Name: "ssh", Data: "ssh"},
		{Name: "ftp", Data: "ftp"},
		{Name: "parent/v1", Data: "parent-v1"},
		{Name: "parent/v2", Data: "parent-v2"},
		{Name: "parent/v3", Data: "parent-v3"},
	})
	require.Len(t, BuildInBruteTypeTree, 3)
	require.Equal(t, "ssh", BuildInBruteTypeTree[0].Name)
	require.Equal(t, "ssh", BuildInBruteTypeTree[0].Data)

	require.Len(t, BuildInBruteTypeTree[2].Children, 3)
	require.Equal(t, BuildInBruteTypeTree[2].Children[0].Name, "v1")
	require.Equal(t, BuildInBruteTypeTree[2].Children[0].Data, "parent-v1")

	require.Equal(t, BuildInBruteTypeTree[2].Children[2].Name, "v3")
	require.Equal(t, BuildInBruteTypeTree[2].Children[2].Data, "parent-v3")
}
