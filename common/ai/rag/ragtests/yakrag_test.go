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

//go:embed no-audio-mov.mov
var noAudioVideoData []byte

func TestRAGFromYaklang(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip github action")
		return
	}

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

// TestRAGFromYaklang_NoAudioVideo tests RAG knowledge base building from a video without audio track
// This test verifies that the system can gracefully handle videos without audio and fallback to
// scene change detection + fixed interval frame extraction strategy
func TestRAGFromYaklang_NoAudioVideo(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip github action")
		return
	}

	yakit.LoadGlobalNetworkConfig()

	// Write embedded video data to a temp file
	filename := consts.TempFileFast(string(noAudioVideoData), "no-audio-*.mov")
	defer os.RemoveAll(filename)

	if !utils.FileExists(filename) {
		t.Fatalf("Test video file %s does not exist", filename)
	}

	script := yak.NewScriptEngine(1)
	engine, err := script.ExecuteEx(`
println("开始构建无音轨视频的RAG索引...")
filename = getParam("filename")
println("fetch no-audio video file: %v" % filename)

kbName = "demo_kb_no_audio_" + randstr(10)
defer rag.DeleteRAG(kbName) 

// Build knowledge base from video file - should handle no audio track gracefully
rag.BuildIndexKnowledgeFromFile(kbName, filename)~

println("Successfully built RAG index from no-audio video")
`, map[string]any{
		"filename": filename,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = engine
}
