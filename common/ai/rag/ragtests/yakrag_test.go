package ragtests

import (
	"os"
	"testing"

	_ "embed"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	_ "github.com/yaklang/yaklang/common/ai/rag"
)

//go:embed testdata.txt
var demo string

func TestRAGFromYaklang(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()

	filename := consts.TempFileFast(demo)
	defer os.RemoveAll(filename)

	if !utils.FileExists(filename) {
		t.Fatalf("File %s does not exist", filename)
	}

	script := yak.NewScriptEngine(1)
	engine, err := script.ExecuteEx(`
println("开始构建RAG索引...")
filename = getParam("filename")
println("fetch demo file: %v" % filename)

kbName = "demo_kb_" + randstr(10)
defer rag.DeleteRAG(kbName) 
rag.BuildIndexKnowledgeFromFile(kbName, filename)~

`, map[string]any{
		"filename": filename,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = engine
}
