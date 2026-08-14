package loop_http_fuzztest

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func newHTTPFuzztestLoopForDecryptTest(t *testing.T) *reactloops.ReActLoop {
	t.Helper()
	invoker := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create http_fuzztest loop: %v", err)
	}
	return loop
}

func executeDecryptDataAction(t *testing.T, loop *reactloops.ReActLoop, params map[string]any) *reactloops.LoopActionHandlerOperator {
	t.Helper()
	handler, err := loop.GetActionHandler("decrypt_data")
	if err != nil {
		t.Fatalf("get decrypt_data action: %v", err)
	}

	actionMap := map[string]any{
		"@action": "decrypt_data",
	}
	for key, value := range params {
		actionMap[key] = value
	}
	rawAction, err := json.Marshal(actionMap)
	if err != nil {
		t.Fatalf("marshal action: %v", err)
	}
	action, err := aicommon.ExtractAction(string(rawAction), "decrypt_data")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := handler.ActionVerifier(loop, action); err != nil {
		t.Fatalf("verify action: %v", err)
	}

	task := aicommon.NewStatefulTaskBase("decrypt-data-test", "decrypt data", context.Background(), loop.GetEmitter())
	operator := reactloops.NewActionHandlerOperator(task)
	handler.ActionHandler(loop, action, operator)
	return operator
}

func TestDecryptDataAction_AESCBCBase64PKCS7(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	key := []byte("1234567890123456")
	iv := []byte("abcdefghijklmnop")
	ciphertext, err := codec.AESEncryptCBCWithPKCSPadding(key, []byte("hello yaklang"), iv)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":       "aes-cbc",
		"key":             string(key),
		"iv":              string(iv),
		"data":            codec.EncodeBase64(ciphertext),
		"data_encoding":   "base64",
		"padding":         "pkcs7",
		"output_encoding": "utf8",
		"reason":          "测试 AES-CBC 解密。",
	})

	if got := loop.Get(loopHTTPFuzzDecryptResultKey); got != "hello yaklang" {
		t.Fatalf("expected decrypted 'hello yaklang', got %q", got)
	}
	if errStr := loop.Get(loopHTTPFuzzDecryptErrorKey); errStr != "" {
		t.Fatalf("expected no decrypt error, got %q", errStr)
	}
	// hex 与 base64 结果也应正确落库
	if loop.Get(loopHTTPFuzzDecryptResultHexKey) != codec.EncodeToHex([]byte("hello yaklang")) {
		t.Fatalf("unexpected hex result: %s", loop.Get(loopHTTPFuzzDecryptResultHexKey))
	}
	if loop.Get(loopHTTPFuzzDecryptResultBase64Key) != codec.EncodeBase64([]byte("hello yaklang")) {
		t.Fatalf("unexpected base64 result: %s", loop.Get(loopHTTPFuzzDecryptResultBase64Key))
	}
}

func TestDecryptDataAction_AESECBHexKeyHexData(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	key := []byte("0123456789abcdef")
	ciphertext, err := codec.AESEncryptECBWithPKCSPadding(key, []byte("ecb-mode-test"), nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":       "aes-ecb",
		"key":             codec.EncodeToHex(key),
		"key_encoding":    "hex",
		"data":            codec.EncodeToHex(ciphertext),
		"data_encoding":   "hex",
		"padding":         "pkcs7",
		"output_encoding": "utf8",
		"reason":          "测试 AES-ECB 解密。",
	})

	if got := loop.Get(loopHTTPFuzzDecryptResultKey); got != "ecb-mode-test" {
		t.Fatalf("expected decrypted 'ecb-mode-test', got %q", got)
	}
}

func TestDecryptDataAction_DESECBZeroPadding(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	key := []byte("12345678")
	// 构造 DES-ECB 零填充密文：data 长度自动补 0 到 8 字节倍数
	raw := []byte("des-data")
	if len(raw)%8 != 0 {
		raw = codec.ZeroPadding(raw, 8)
	}
	ciphertext, err := codec.DESEnc(key, raw, nil, codec.ECB)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":       "des-ecb",
		"key":             string(key),
		"data":            codec.EncodeToHex(ciphertext),
		"data_encoding":   "hex",
		"padding":         "zero",
		"output_encoding": "utf8",
		"reason":          "测试 DES-ECB 零填充解密。",
	})

	if got := loop.Get(loopHTTPFuzzDecryptResultKey); got != "des-data" {
		t.Fatalf("expected decrypted 'des-data', got %q", got)
	}
}

func TestDecryptDataAction_InvalidAlgorithmRejected(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	handler, err := loop.GetActionHandler("decrypt_data")
	if err != nil {
		t.Fatalf("get decrypt_data action: %v", err)
	}
	action, err := aicommon.ExtractAction(`{"@action":"decrypt_data","algorithm":"unknown-x","key":"k","data":"ZGF0YQ==","reason":"r"}`, "decrypt_data")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := handler.ActionVerifier(loop, action); err == nil {
		t.Fatal("expected verifier to reject unknown algorithm")
	}
}

func TestDecryptDataAction_MissingDataRejected(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	handler, err := loop.GetActionHandler("decrypt_data")
	if err != nil {
		t.Fatalf("get decrypt_data action: %v", err)
	}
	action, err := aicommon.ExtractAction(`{"@action":"decrypt_data","algorithm":"aes-cbc","key":"k","reason":"r"}`, "decrypt_data")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := handler.ActionVerifier(loop, action); err == nil {
		t.Fatal("expected verifier to reject missing data")
	}
}

func TestDecryptDataAction_NonUTF8OutputFallsBackToHex(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	key := []byte("1234567890123456")
	iv := []byte("abcdefghijklmnop")
	// 二进制明文：包含非 UTF-8 字节（0xff 0xfe 等），不是合法 UTF-8
	bin := []byte{0xff, 0xfe, 0xfd, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d}
	ciphertext, err := codec.AESEncryptCBCWithPKCSPadding(key, bin, iv)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":       "aes-cbc",
		"key":             string(key),
		"iv":              string(iv),
		"data":            codec.EncodeBase64(ciphertext),
		"data_encoding":   "base64",
		"padding":         "pkcs7",
		"output_encoding": "utf8",
		"reason":          "测试非 UTF-8 明文输出回退。",
	})

	result := loop.Get(loopHTTPFuzzDecryptResultKey)
	if !strings.Contains(result, "hex") {
		t.Fatalf("expected non-utf8 output to fall back to hex, got %q", result)
	}
	if !strings.Contains(result, codec.EncodeToHex(bin)) {
		t.Fatalf("expected hex representation of plaintext in output, got %q", result)
	}
}

func TestDecryptDataAction_DecryptFailureSetsError(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	// 用错误长度的 key 触发失败
	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":     "aes-cbc",
		"key":           "short",
		"iv":            "abcdefghijklmnop",
		"data":          "AAAA",
		"data_encoding": "base64",
		"padding":       "pkcs7",
		"reason":        "测试错误 key 长度导致解密失败。",
	})

	if errStr := loop.Get(loopHTTPFuzzDecryptErrorKey); errStr == "" {
		t.Fatal("expected decrypt error to be recorded")
	}
	if got := loop.Get(loopHTTPFuzzDecryptResultKey); got != "" {
		t.Fatalf("expected no decrypt result on failure, got %q", got)
	}
}

func TestDecryptDataAction_SM4CBC(t *testing.T) {
	loop := newHTTPFuzztestLoopForDecryptTest(t)
	key := []byte("0123456789abcdef")
	iv := []byte("abcdefghijklmnop")
	ciphertext, err := codec.SM4EncryptCBCWithPKCSPadding(key, []byte("sm4-cbc-test"), iv)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":       "sm4-cbc",
		"key":             string(key),
		"iv":              string(iv),
		"data":            codec.EncodeBase64(ciphertext),
		"data_encoding":   "base64",
		"padding":         "pkcs7",
		"output_encoding": "utf8",
		"reason":          "测试 SM4-CBC 解密。",
	})

	if got := loop.Get(loopHTTPFuzzDecryptResultKey); got != "sm4-cbc-test" {
		t.Fatalf("expected decrypted 'sm4-cbc-test', got %q", got)
	}
}

func TestLooksLikeLoopHTTPFuzzNonFuzzDataTask(t *testing.T) {
	cases := map[string]bool{
		"帮我解密这段数据，key 是 abc123":                     true,
		"decrypt this AES-CBC ciphertext with key":  true,
		"把这段密文用 RSA 私钥解出来":                          true,
		"请帮我解析这个 JWT":                               true,
		"帮我 fuzz 一下这个 URL: http://example.com?id=1": false,
		"GET /login HTTP/1.1\nHost: example.com":    false,
		"今天天气怎么样":                                   false,
		"帮我看看这个 hockey 比赛的比分":                       false,
		"monkey 和 donkey 有什么区别":                     false,
	}
	for input, want := range cases {
		if got := looksLikeLoopHTTPFuzzNonFuzzDataTask(input); got != want {
			t.Fatalf("looksLikeLoopHTTPFuzzNonFuzzDataTask(%q) = %v, want %v", input, got, want)
		}
	}
}

type decryptActionTestInvoker struct {
	*mock.MockInvoker
	artifactDir    string
	events         []*schema.AiOutputEvent
	resultPayloads []string
	mu             sync.Mutex
}

func newDecryptActionTestInvoker(t *testing.T) *decryptActionTestInvoker {
	t.Helper()
	invoker := &decryptActionTestInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		artifactDir: t.TempDir(),
	}
	if cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig); ok {
		cfg.Emitter = aicommon.NewEmitter("http-fuzztest-decrypt-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			invoker.mu.Lock()
			defer invoker.mu.Unlock()
			invoker.events = append(invoker.events, e)
			return e, nil
		})
	}
	return invoker
}

func (i *decryptActionTestInvoker) EmitResultAfterStream(result any) {
	i.mu.Lock()
	i.resultPayloads = append(i.resultPayloads, strings.TrimSpace(utils.InterfaceToString(result)))
	i.mu.Unlock()
	if cfg, ok := i.GetConfig().(*mock.MockedAIConfig); ok && cfg.Emitter != nil {
		_, _ = cfg.Emitter.EmitResultAfterStream("result", result, false)
	}
}

func (i *decryptActionTestInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return i.artifactDir
}

func TestDecryptDataAction_EmitsMarkdownResult(t *testing.T) {
	invoker := newDecryptActionTestInvoker(t)
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}

	key := []byte("1234567890123456")
	iv := []byte("abcdefghijklmnop")
	ciphertext, err := codec.AESEncryptCBCWithPKCSPadding(key, []byte("visible-markdown-result"), iv)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	executeDecryptDataAction(t, loop, map[string]any{
		"algorithm":       "aes-cbc",
		"key":             string(key),
		"iv":              string(iv),
		"data":            codec.EncodeBase64(ciphertext),
		"data_encoding":   "base64",
		"padding":         "pkcs7",
		"output_encoding": "utf8",
		"reason":          "验证解密结果以 markdown 交付。",
	})

	if emitter := loop.GetEmitter(); emitter != nil {
		emitter.WaitForStream()
	}

	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected one result payload, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "## 解密结果") {
		t.Fatalf("expected markdown result header, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "visible-markdown-result") {
		t.Fatalf("expected decrypted plaintext in result, got: %s", invoker.resultPayloads[0])
	}
	var sawMarkdownStream bool
	for _, event := range invoker.events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream {
			sawMarkdownStream = true
			break
		}
	}
	if !sawMarkdownStream {
		t.Fatal("expected decrypt_data to emit markdown stream")
	}
}
