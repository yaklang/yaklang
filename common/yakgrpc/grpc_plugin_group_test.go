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
	t.Run("SaveToExistingGroup", func(t *testing.T) {
		_, err = client.SaveYakScriptGroup(context.Background(), &ypb.SaveYakScriptGroupRequest{
			Filter: &ypb.QueryYakScriptRequest{
				IncludedScriptNames: []string{"基础 XSS 检测"},
			},
			SaveGroup:   []string{"测试分组1", "测试分组2"},
			RemoveGroup: nil,
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("RemoveFromGroup", func(t *testing.T) {
		_, err = client.SaveYakScriptGroup(context.Background(), &ypb.SaveYakScriptGroupRequest{
			Filter: &ypb.QueryYakScriptRequest{
				IncludedScriptNames: []string{"基础 XSS 检测"},
			},
			SaveGroup:   nil,
			RemoveGroup: []string{"测试分组1"},
		})
		if err != nil {
			t.Fatal(err)
		}
	})

}

func TestRenameYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("RenameExistingScriptGroup", func(t *testing.T) {
		rsp, err := client.RenameYakScriptGroup(context.Background(), &ypb.RenameYakScriptGroupRequest{
			Group:    "测试分组1",
			NewGroup: "测试分组3",
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = rsp
	})
	t.Run("RenameNonExistentScriptGroupError", func(t *testing.T) {
		rsp, err := client.RenameYakScriptGroup(context.Background(), &ypb.RenameYakScriptGroupRequest{
			Group:    "",
			NewGroup: "新分组",
		})
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		_ = rsp
	})
}

func TestDeleteYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("DeleteExistingScriptGroup", func(t *testing.T) {
		_, err = client.DeleteYakScriptGroup(context.Background(), &ypb.DeleteYakScriptGroupRequest{Group: "测试分组2"})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("DeleteNonExistentScriptGroup", func(t *testing.T) {
		rsp, err := client.DeleteYakScriptGroup(context.Background(), &ypb.DeleteYakScriptGroupRequest{Group: ""})
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		_ = rsp
	})
}

func TestGetYakScriptGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("GetValidScriptGroup", func(t *testing.T) {
		_, err = client.GetYakScriptGroup(context.Background(), &ypb.QueryYakScriptRequest{
			IncludedScriptNames: nil,
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("GetSpecificScriptGroup", func(t *testing.T) {
		_, err = client.GetYakScriptGroup(context.Background(), &ypb.QueryYakScriptRequest{
			IncludedScriptNames: []string{"基础 XSS 检测"},
		})
		if err != nil {
			t.Fatal(err)
		}
	})

}

func TestResetYakScriptGroup(t *testing.T) {
	testCases := []struct {
		name     string
		token    string
		expected bool // Whether the function should return an error
	}{
		{
			name:     "Valid token",
			token:    "",
			expected: false,
		},
		{
			name:     "Invalid token",
			token:    "77_29ekIsIgIL7j8m3XgHP9-XiqKEwKDfNTGgN0D5m4yB70JbIAxDhI5Vgh4OEsuj--cVWiUbBEctRPkdhBIhreRLL93v9woLQrgA-xWuQkBU8",
			expected: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewLocalClient()
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ResetYakScriptGroup(context.Background(), &ypb.ResetYakScriptGroupRequest{Token: tc.token})
			if tc.expected && err == nil {
				//t.Fatal("expected an error, but got nil")
			}
			if !tc.expected && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSetGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("SetGroup", func(t *testing.T) {
		_, err = client.SetGroup(context.Background(), &ypb.SetGroupRequest{GroupName: "测试组"})
		if err != nil {
			t.Fatal(err)
		}
	})
}
