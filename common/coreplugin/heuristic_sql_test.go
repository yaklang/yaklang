package coreplugin

import (
	"context"
	"github.com/yaklang/yaklang/common/vulinbox"
	"testing"
)

func TestGRPCMUSTPASS_SQL(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "启发式SQL注入检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
	vul1 := VulInfo{
		Path:           "/user/by-id-safe?id=1",
		ExpectedResult: map[string]int{"参数:id未检测到闭合边界": 3},
	}
	vul2 := VulInfo{
		Path:           "/user/id?id=1",
		ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字型[id] 无边界闭合】": 3, "存在基于UNION SQL 注入: [参数名:id 原值:1]": 3},
	}
	vul3 := VulInfo{
		Path:           "/user/id-json?id=%7B%22uid%22%3A1%2C%22id%22%3A%221%22%7D",
		ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字型[id] 无边界闭合】": 3, "存在基于UNION SQL 注入: [参数名:id 原值:1]": 3},
	}
	vul4 := VulInfo{
		Path:           "/user/id-b64-json?id=eyJ1aWQiOjEsImlkIjoiMSJ9",
		ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字型[id] 无边界闭合】": 3, "存在基于UNION SQL 注入: [参数名:id 原值:1]": 3},
	}
	vul5 := VulInfo{
		Path:           "/user/name?name=admin",
		ExpectedResult: map[string]int{"疑似SQL注入：【参数：字符串[name] 单引号闭合】": 3, "存在基于UNION SQL 注入: [参数名:name 原值:admin]": 3},
	}

	Must(TestCoreMitmPlug(pluginName, server, vul1, client, t), "SQL插件对于安全的SQL注入检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul2, client, t), "SQL插件对于不安全的SQL注入(ID)检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul3, client, t), "SQL插件对于不安全的SQL注入(JSON-ID)检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul4, client, t), "SQL插件对于不安全的SQL注入(BASE64-JSON-ID)检测结果不符合预期")
	Must(TestCoreMitmPlug(pluginName, server, vul5, client, t), "SQL插件对于不安全的SQL注入(NAME)检测结果不符合预期")
}
