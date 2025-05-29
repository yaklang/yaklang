package yakgrpc

import (
	"context"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

func TestQueryGroupCount(t *testing.T) {
	scriptName2, clearFunc, _ := yakit.CreateTemporaryYakScriptEx("mitm", "")
	scriptName1 := "[TMP]-" + "-" + ksuid.New().String()
	content := "passiveScanning = cli.String(\"passiveScanning\",cli.setHelp(\"填写规则:http://127.0.0.1:8080\"),cli.setRequired(true),cli.setVerboseName(\"被动扫描器地址\"))\nwhitelisthostcli = cli.String(\"whitelisthostcli\",cli.setHelp(\"格式:*.yaklang.com;10.0.0.* 中间用;分割\"),cli.setDefault(\"\"),cli.setVerboseName(\"白名单主机列表\"))\nblacklistcli = cli.String(\"blacklistcli\",cli.setHelp(\"默认:*.gov.cn;*.edu.cn 中间用;分割\"),cli.setDefault(\"*.gov.cn;*.edu.cn\"),cli.setVerboseName(\"黑名单主机列表\"))\nblackmethodcli = cli.StringSlice(\"blackmethodcli\",cli.setMultipleSelect(true),\ncli.setSelectOption(\"GET\", \"GET\"),\ncli.setSelectOption(\"POST\", \"POST\"),\ncli.setSelectOption(\"DELETE\", \"DELETE\"),\ncli.setSelectOption(\"PUT\", \"PUT\"),\ncli.setSelectOption(\"OPTIONS\", \"OPTIONS\"),\ncli.setSelectOption(\"TRACE\", \"TRACE\"),\ncli.setSelectOption(\"COPY\", \"COPY\"),cli.setVerboseName(\"禁用方法\")\n)\ncli.check()\n"
	prog, err := static_analyzer.SSAParse(content, "mitm")
	if err != nil {
		t.Fatal("ssa parse error")
	}
	parameters, _, _ := information.ParseCliParameter(prog)
	params := information.CliParam2grpc(parameters)
	err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), scriptName1, GRPCYakScriptToYakitScript(&ypb.YakScript{
		Content:    content,
		Type:       "mitm",
		Params:     params,
		ScriptName: scriptName1,
	}))
	if err != nil {
		t.Fatal("create yakscript error")
	}
	group1 := scriptName1 + "-" + "group1"
	group2 := scriptName2 + "-" + "group2"
	saveData1 := &schema.PluginGroup{
		YakScriptName: scriptName1,
		Group:         group1,
	}
	saveData1.Hash = saveData1.CalcHash()
	saveData2 := &schema.PluginGroup{
		YakScriptName: scriptName2,
		Group:         group2,
	}
	saveData2.Hash = saveData2.CalcHash()
	yakit.CreateOrUpdatePluginGroup(consts.GetGormProfileDatabase(), saveData1.Hash, saveData1)
	yakit.CreateOrUpdatePluginGroup(consts.GetGormProfileDatabase(), saveData2.Hash, saveData2)

	defer func() {
		clearFunc()
		if err = yakit.DeletePluginGroupByScriptName(consts.GetGormProfileDatabase(), []string{scriptName2, scriptName1}); err != nil {
			t.Errorf("failed to delete plugin groups: %v", err)
		}
		if err = yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), scriptName1); err != nil {
			t.Errorf("failed to delete YakScript: %v", err)
		}
	}()

	tests := []struct {
		name               string
		excludeType        []string
		isMITMParamPlugins int64
		expectedError      error
		yakScriptName      string
	}{
		{
			name:               "Case 1: isMITMParamPlugins=1",
			excludeType:        []string{},
			isMITMParamPlugins: 1,
			expectedError:      nil,
		},
		{
			name:               "Case 2: isMITMParamPlugins=2",
			excludeType:        []string{},
			isMITMParamPlugins: 2,
			expectedError:      nil,
		},
		{
			name:               "Case 0: No Params Filtering",
			excludeType:        []string{},
			isMITMParamPlugins: 0,
			expectedError:      nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := yakit.QueryGroupCount(consts.GetGormProfileDatabase(), tc.excludeType, tc.isMITMParamPlugins)
			if err != nil {
				t.Fatal(err)
			}
			var expectedValue string
			var expectedCount int
			switch tc.isMITMParamPlugins {
			case 1:
				expectedValue = group1
				expectedCount = 1
			case 2:
				expectedValue = group2
				expectedCount = 1
			default:
				return
			}
			var valueFound bool
			for _, v := range req {
				if v.Value == expectedValue && v.Count == expectedCount {
					valueFound = true
					break
				}
			}
			if !valueFound {
				t.Errorf("Expected group %s with count %d was not found for isMITMParamPlugins=%d", expectedValue, expectedCount, tc.isMITMParamPlugins)
			}
		})

	}
}
