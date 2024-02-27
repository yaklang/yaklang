package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestQueryYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SaveYakScriptGroup(context.Background(), &ypb.SaveYakScriptGroupRequest{
		Filter: &ypb.QueryYakScriptRequest{
			IncludedScriptNames: []string{"基础 XSS 检测"},
		},
		SaveGroup:   []string{"测试分组1", "测试分组2"},
		RemoveGroup: nil,
	})
	rsp, err := client.QueryYakScriptGroup(context.Background(), &ypb.QueryYakScriptGroupRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
}

func TestSaveYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.SaveYakScriptGroup(context.Background(), &ypb.SaveYakScriptGroupRequest{
		Filter: &ypb.QueryYakScriptRequest{
			IncludedScriptNames: []string{"基础 XSS 检测"},
		},
		SaveGroup:   []string{"测试分组1", "测试分组2"},
		RemoveGroup: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
}

func TestRenameYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.RenameYakScriptGroup(context.Background(), &ypb.RenameYakScriptGroupRequest{
		Group:    "测试分组1",
		NewGroup: "测试分组3",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
}

func TestDeleteYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.DeleteYakScriptGroup(context.Background(), &ypb.DeleteYakScriptGroupRequest{Group: "测试分组2"})
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
}

func TestGetYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.GetYakScriptGroup(context.Background(), &ypb.QueryYakScriptRequest{
		IncludedScriptNames: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
}

func TestResetYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.ResetYakScriptGroup(context.Background(), &ypb.ResetYakScriptGroupRequest{Token: ""})
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
}
