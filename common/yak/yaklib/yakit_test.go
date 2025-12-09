package yaklib

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-rod/rod/lib/utils"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYakitServer_Addr(t *testing.T) {
	s := NewYakitServer(2335, SetYakitServer_ProgressHandler(func(id string, progress float64) {
		log.Infof("progress: %v  - percent float: %v", id, progress)
	}))
	go func() {
		s.Start()
	}()
	time.Sleep(time.Second * 1)

	spew.Dump(s.Addr())
	c := NewYakitClient(s.Addr())
	c.YakitSetProgressEx("test", 0.5)
	c.YakitSetProgressEx("test", 0.6)
	c.YakitSetProgressEx("test", 0.7)
	c.YakitSetProgressEx("test", 0.8)
	c.YakitSetProgressEx("test", 0.99)

	time.Sleep(time.Second)
}

func TestMUSTPASS_YakitLog(t *testing.T) {
	randStr := utils.RandString(20)
	testCases := []struct {
		name     string
		format   string
		input    string
		expected string
	}{
		{"Test1", "%%d %s", randStr, "%d " + randStr},
		{"Test2", "%s", randStr, randStr},
		{"Test3", "%x", randStr, hex.EncodeToString([]byte(randStr))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			GetExtYakitLibByClient(NewVirtualYakitClient(func(i *ypb.ExecResult) error {
				bb := jsonpath.Find(i.GetMessage(), "$.content.data")
				if bb != tc.expected {
					t.Fatalf("expect: %s, got: %s", tc.expected, bb)
				}
				return nil
			}))["Info"].(func(tmp string, items ...interface{}))(tc.format, tc.input)
		})
	}
}

// TestMUSTPASS_ConvertExecResultIntoAIToolCallStdoutLog
// 要求展示：Text/Success/Code/Markdown/Report/Error/Info/Warn/Stream
// 不展示：File/Debug/Progress/StatusCard 以及 file-level 的 log
func TestMUSTPASS_ConvertExecResultIntoAIToolCallStdoutLog(t *testing.T) {
	var mu sync.Mutex
	var outputs []string

	client := NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		out := ConvertExecResultIntoAIToolCallStdoutLog(result)
		mu.Lock()
		outputs = append(outputs, out)
		mu.Unlock()
		return nil
	}, "runtime-test")

	// should be displayed
	client.YakitTextBlock("text-block")
	client.YakitSuccess("success-content")
	client.YakitCode("code-content")
	client.YakitMarkdown("markdown-content")
	client.YakitReport(42)
	client.YakitError("error-content")
	client.YakitInfo("info-content")
	client.YakitWarn("warn-content")
	client.Stream("stdout", "stream-1", strings.NewReader("stream-data"))

	// should NOT be displayed
	client.YakitFile("/tmp/fake.txt")
	client.YakitDebug("debug-content")
	client.YakitSetProgress(0.5)
	client.YakitSetProgressEx("progress-id", 0.6)

	// status-card message should be hidden
	rawStatus, err := YakitMessageGenerator(&YakitStatusCard{Id: "status-1", Data: "status-data"})
	if err != nil {
		t.Fatalf("failed to generate status card message: %v", err)
	}
	outputs = append(outputs, ConvertExecResultIntoAIToolCallStdoutLog(&ypb.ExecResult{
		IsMessage: true,
		Message:   rawStatus,
	}))

	// wait a bit to ensure stream logs flushed
	time.Sleep(50 * time.Millisecond)

	joined := strings.Join(outputs, "\n")

	shouldContains := []string{
		"[text] text-block",
		"[success] success-content",
		"[code] code-content",
		"[markdown] markdown-content",
		"[report] 42",
		"[error] error-content",
		"[info] info-content",
		"[warn] warn-content",
		"[stream]",
	}
	for _, want := range shouldContains {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected stdout log to contain %q, got: %s", want, joined)
		}
	}

	shouldNotContains := []string{
		"[file]",
		"debug-content",
		"progress-id",
		"status-data",
	}
	for _, bad := range shouldNotContains {
		if strings.Contains(joined, bad) {
			t.Fatalf("expected stdout log to NOT contain %q, got: %s", bad, joined)
		}
	}
}
