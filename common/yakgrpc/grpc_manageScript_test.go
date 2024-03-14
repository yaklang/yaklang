package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestQueryYakScriptRiskDetailByCWE(t *testing.T) {
	test := assert.New(t)
	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	_, err = client.QueryYakScriptRiskDetailByCWE(context.Background(), &ypb.QueryYakScriptRiskDetailByCWERequest{CWEId: "502"})
	if err != nil {
		panic(err)
	}
}

func TestYakScriptRiskTypeList(t *testing.T) {
	test := assert.New(t)
	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	_, err = client.YakScriptRiskTypeList(context.Background(), &ypb.Empty{})

}

func TestImportYakScript(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	s, err := client.ImportYakScript(context.Background(), &ypb.ImportYakScriptRequest{Dirs: []string{"/Users/limin/Downloads/yak_script"}})
	if err != nil {
		test.FailNow(err.Error())
	}
	_ = s
}

func TestExportLocalYakScriptStream(t *testing.T) {
	test := assert.New(t)

	client, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}
	s, err := client.ExportLocalYakScriptStream(context.Background(), &ypb.ExportLocalYakScriptRequest{
		OutputDir:       "/Users/limin/Downloads/",
		OutputPluginDir: "",
		YakScriptIds:    nil,
		Keywords:        "",
		Type:            "",
		UserName:        "",
		Tags:            "",
	})
	if err != nil {
		test.FailNow(err.Error())
	}
	_ = s
}

func TestTempYakScriptQuery(t *testing.T) {
	scriptName, err := yakit.CreateTemporaryYakScript("yak", "")
	if err != nil {
		t.Fatal(err)
	}
	defer yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), scriptName)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.QueryYakScript(context.Background(), &ypb.QueryYakScriptRequest{
		Keyword:  scriptName,
		IsIgnore: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Data) != 1 {
		t.Fatalf("just keyword query err, len(res)[%d] != 1", len(res.Data))
	}

	res, err = client.QueryYakScript(context.Background(), &ypb.QueryYakScriptRequest{
		Keyword: scriptName,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) != 0 {
		t.Fatalf("ingore is ineffective, len(res)[%d] != 0", len(res.Data))
	}
}
