package loop_http_fuzztest

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	loopHTTPFuzzDecryptContentField      = "decrypt_data_content"
	loopHTTPFuzzDecryptResultKey         = "decrypt_data_result"
	loopHTTPFuzzDecryptResultHexKey      = "decrypt_data_result_hex"
	loopHTTPFuzzDecryptResultBase64Key   = "decrypt_data_result_base64"
	loopHTTPFuzzDecryptErrorKey          = "decrypt_data_error"
	loopHTTPFuzzDecryptSummaryKey        = "decrypt_data_summary"
	loopHTTPFuzzDecryptAITagNode         = "decrypt-data-content"
	loopHTTPFuzzDecryptResultMarkdownKey = "decrypt_data_result_markdown"
)

// loopHTTPFuzzDecryptSpec 描述一次离线数据解密请求的完整参数。
type loopHTTPFuzzDecryptSpec struct {
	Algorithm      string // aes-cbc / aes-ecb / aes-cfb / aes-ofb / aes-ctr / aes-gcm / des-cbc / des-ecb / 3des-cbc / 3des-ecb / rc4 / sm4-cbc / sm4-ecb / rsa
	Key            []byte
	KeyEncoding    string // utf8 / hex / base64
	IV             []byte
	IVEncoding     string
	Data           []byte
	DataEncoding   string // utf8 / hex / base64
	Padding        string // pkcs7 / zero / none
	OutputEncoding string // utf8 / hex / base64
}

var decryptDataAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"decrypt_data",
		"使用给定的密钥和算法对加密数据进行离线解密，适用于用户提供密文 + 密钥并要求解密的场景（如数据包中的加密参数、Cookie、Body 字段等）。解密过程不发送任何请求，只做本地计算。",
		[]aitool.ToolOption{
			aitool.WithStringParam("algorithm", aitool.WithParam_Description("解密算法：aes-cbc / aes-ecb / aes-cfb / aes-ofb / aes-ctr / aes-gcm / des-cbc / des-ecb / 3des-cbc / 3des-ecb / rc4 / sm4-cbc / sm4-ecb / rsa"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("key", aitool.WithParam_Description("解密密钥。默认按 UTF-8 字符串解释；也可以通过 key_encoding 指定 hex/base64 编码。RSA 场景下这里是 PEM 私钥全文。"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("key_encoding", aitool.WithParam_Description("密钥编码：utf8 / hex / base64，默认 utf8。")),
			aitool.WithStringParam("iv", aitool.WithParam_Description("IV/Nonce。仅 CBC/CFB/OFB/CTR/GCM 模式需要；ECB/RC4/RSA 不需要。")),
			aitool.WithStringParam("iv_encoding", aitool.WithParam_Description("IV 编码：utf8 / hex / base64，默认 utf8。")),
			aitool.WithStringParam("data", aitool.WithParam_Description("待解密的密文。支持通过 DECRYPT_DATA AITAG 传递大段密文；data 与 AITAG 至少提供一个。"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("data_encoding", aitool.WithParam_Description("密文编码：base64 / hex / utf8，默认 base64。数据包中最常见的密文是 base64。")),
			aitool.WithStringParam("padding", aitool.WithParam_Description("填充模式：pkcs7 / zero / none，默认 pkcs7。流模式（OFB/CTR/CFB）和 GCM/RC4/RSA 无需填充，可忽略。")),
			aitool.WithStringParam("output_encoding", aitool.WithParam_Description("输出编码：utf8 / hex / base64，默认 utf8。若明文不是合法 UTF-8，会自动回退为 hex 输出。")),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "reason", AINodeId: "thought"},
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			spec, err := parseLoopHTTPFuzzDecryptSpec(action, loop)
			if err != nil {
				return err
			}
			// 预校验：算法+密钥长度是否合法，避免进入 handler 后才失败。
			if err := validateLoopHTTPFuzzDecryptSpec(spec); err != nil {
				return err
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			spec, err := parseLoopHTTPFuzzDecryptSpec(action, loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			paramSummary := buildLoopHTTPFuzzDecryptParamSummary(spec)
			log.Infof("decrypt_data action: %s", paramSummary)

			reactloops.EmitActionLog(loop, loopHTTPFuzzActionLogNodeSetRequest, fmt.Sprintf("解密数据: %s", spec.Algorithm))
			reactloops.EmitStatusI18n(loop, "解密中", "Decrypting...")

			plainBytes, err := decryptLoopHTTPFuzzData(spec)
			if err != nil {
				loop.Set(loopHTTPFuzzDecryptErrorKey, err.Error())
				feedback := fmt.Sprintf("解密失败：%v\n\n请检查 algorithm / key / iv / padding / 编码参数是否匹配实际加密方式。", err)
				record := recordLoopHTTPFuzzMetaAction(loop, "decrypt_data", paramSummary, utils.ShrinkTextBlock(feedback, 240))
				r.AddToTimeline("decrypt_data", fmt.Sprintf("Decrypt failed: %s\n%s", spec.Algorithm, err.Error()))
				persistLoopHTTPFuzzSessionContext(loop, "decrypt_data_failed")
				reactloops.EmitStatusI18n(loop, "解密失败", "Decrypt Failed")
				operator.Feedback(buildLoopHTTPFuzzActionFeedback(record) + "\n\n" + feedback)
				return
			}

			outputText := encodeLoopHTTPFuzzDecryptOutput(plainBytes, spec.OutputEncoding)
			loop.Set(loopHTTPFuzzDecryptResultKey, outputText)
			loop.Set(loopHTTPFuzzDecryptResultHexKey, codec.EncodeToHex(plainBytes))
			loop.Set(loopHTTPFuzzDecryptResultBase64Key, codec.EncodeBase64(plainBytes))
			loop.Set(loopHTTPFuzzDecryptErrorKey, "")

			resultMarkdown := buildLoopHTTPFuzzDecryptResultMarkdown(spec, plainBytes, outputText)
			loop.Set(loopHTTPFuzzDecryptResultMarkdownKey, resultMarkdown)
			loop.Set(loopHTTPFuzzDecryptSummaryKey, buildLoopHTTPFuzzDecryptParamSummary(spec))

			// 交付结果：markdown 流 + 文件 artifact + result
			invoker := loop.GetInvoker()
			if emitter := loop.GetEmitter(); emitter != nil {
				taskID := ""
				if task := loop.GetCurrentTask(); task != nil {
					taskID = task.GetId()
				}
				if _, err := emitter.EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(resultMarkdown), taskID, func() {}); err != nil {
					log.Warnf("decrypt_data: failed to emit markdown stream: %v", err)
				}
			}
			invoker.EmitFileArtifactWithExt("decrypt_data", ".md", resultMarkdown)
			invoker.EmitResultAfterStream(resultMarkdown)

			record := recordLoopHTTPFuzzMetaAction(loop, "decrypt_data", paramSummary, utils.ShrinkTextBlock(resultMarkdown, 240))
			r.AddToTimeline("decrypt_data", fmt.Sprintf("Decrypted %s, output: %s", spec.Algorithm, utils.ShrinkTextBlock(outputText, 200)))
			persistLoopHTTPFuzzSessionContext(loop, "decrypt_data")
			reactloops.EmitStatusI18n(loop, "完成", "Complete")
			reactloops.EmitActionLog(loop, loopHTTPFuzzActionLogNodeSetRequest, "解密完成 / Decrypt Complete", utils.ShrinkTextBlock(outputText, 1200))
			operator.Feedback(buildLoopHTTPFuzzActionFeedback(record) + "\n\n" + resultMarkdown)
		},
	)
}

// parseLoopHTTPFuzzDecryptSpec 从 action 中解析解密参数，兼容 data 参数与 DECRYPT_DATA AITAG 两种来源。
func parseLoopHTTPFuzzDecryptSpec(action *aicommon.Action, loop *reactloops.ReActLoop) (*loopHTTPFuzzDecryptSpec, error) {
	if action == nil {
		return nil, fmt.Errorf("decrypt_data action is nil")
	}

	algorithm := strings.ToLower(strings.TrimSpace(action.GetString("algorithm")))
	if algorithm == "" {
		return nil, fmt.Errorf("algorithm is required")
	}
	keyRaw := action.GetString("key")
	keyEncoding := strings.ToLower(strings.TrimSpace(action.GetString("key_encoding")))
	if keyEncoding == "" {
		keyEncoding = "utf8"
	}
	ivRaw := action.GetString("iv")
	ivEncoding := strings.ToLower(strings.TrimSpace(action.GetString("iv_encoding")))
	if ivEncoding == "" {
		ivEncoding = "utf8"
	}
	dataRaw := strings.TrimSpace(action.GetString("data"))
	if dataRaw == "" {
		dataRaw = strings.TrimSpace(action.GetString(loopHTTPFuzzDecryptContentField))
	}
	if dataRaw == "" && loop != nil {
		dataRaw = strings.TrimSpace(loop.Get(loopHTTPFuzzDecryptContentField))
	}
	if dataRaw == "" {
		return nil, fmt.Errorf("data is required: provide 'data' param or DECRYPT_DATA AITAG content")
	}
	dataEncoding := strings.ToLower(strings.TrimSpace(action.GetString("data_encoding")))
	if dataEncoding == "" {
		dataEncoding = "base64"
	}
	padding := strings.ToLower(strings.TrimSpace(action.GetString("padding")))
	if padding == "" {
		padding = "pkcs7"
	}
	outputEncoding := strings.ToLower(strings.TrimSpace(action.GetString("output_encoding")))
	if outputEncoding == "" {
		outputEncoding = "utf8"
	}

	key, err := decodeLoopHTTPFuzzByteString(keyRaw, keyEncoding)
	if err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
	}
	iv, err := decodeLoopHTTPFuzzByteString(ivRaw, ivEncoding)
	if err != nil {
		return nil, fmt.Errorf("invalid iv: %w", err)
	}
	data, err := decodeLoopHTTPFuzzByteString(dataRaw, dataEncoding)
	if err != nil {
		return nil, fmt.Errorf("invalid data: %w", err)
	}

	return &loopHTTPFuzzDecryptSpec{
		Algorithm:      algorithm,
		Key:            key,
		KeyEncoding:    keyEncoding,
		IV:             iv,
		IVEncoding:     ivEncoding,
		Data:           data,
		DataEncoding:   dataEncoding,
		Padding:        padding,
		OutputEncoding: outputEncoding,
	}, nil
}

// validateLoopHTTPFuzzDecryptSpec 校验算法枚举、编码枚举和 RSA 密钥格式。
func validateLoopHTTPFuzzDecryptSpec(spec *loopHTTPFuzzDecryptSpec) error {
	if spec == nil {
		return fmt.Errorf("decrypt spec is nil")
	}
	validAlgorithms := map[string]struct{}{
		"aes-cbc": {}, "aes-ecb": {}, "aes-cfb": {}, "aes-ofb": {}, "aes-ctr": {}, "aes-gcm": {},
		"des-cbc": {}, "des-ecb": {},
		"3des-cbc": {}, "3des-ecb": {},
		"rc4":     {},
		"sm4-cbc": {}, "sm4-ecb": {},
		"rsa": {},
	}
	if _, ok := validAlgorithms[spec.Algorithm]; !ok {
		return fmt.Errorf("algorithm must be one of: aes-cbc, aes-ecb, aes-cfb, aes-ofb, aes-ctr, aes-gcm, des-cbc, des-ecb, 3des-cbc, 3des-ecb, rc4, sm4-cbc, sm4-ecb, rsa")
	}
	if err := validateLoopHTTPFuzzByteEncoding("key_encoding", spec.KeyEncoding); err != nil {
		return err
	}
	if err := validateLoopHTTPFuzzByteEncoding("iv_encoding", spec.IVEncoding); err != nil {
		return err
	}
	if err := validateLoopHTTPFuzzByteEncoding("data_encoding", spec.DataEncoding); err != nil {
		return err
	}
	if spec.Padding != "pkcs7" && spec.Padding != "zero" && spec.Padding != "none" {
		return fmt.Errorf("padding must be one of: pkcs7, zero, none")
	}
	if spec.OutputEncoding != "utf8" && spec.OutputEncoding != "hex" && spec.OutputEncoding != "base64" {
		return fmt.Errorf("output_encoding must be one of: utf8, hex, base64")
	}
	if spec.Algorithm == "rsa" && len(spec.Key) == 0 {
		return fmt.Errorf("rsa requires a PEM private key in 'key'")
	}
	return nil
}

// decodeLoopHTTPFuzzByteString 按编码把字符串还原为字节。
func decodeLoopHTTPFuzzByteString(raw, encoding string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "hex":
		return codec.DecodeHex(raw)
	case "base64":
		return codec.DecodeBase64(raw)
	case "utf8", "utf-8", "":
		return []byte(raw), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", encoding)
	}
}

func validateLoopHTTPFuzzByteEncoding(name, encoding string) error {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "utf8", "utf-8", "hex", "base64":
		return nil
	default:
		return fmt.Errorf("%s must be one of: utf8, hex, base64", name)
	}
}

// decryptLoopHTTPFuzzData 根据算法把 spec 中的密文解密为明文，并处理填充。
func decryptLoopHTTPFuzzData(spec *loopHTTPFuzzDecryptSpec) ([]byte, error) {
	if spec == nil {
		return nil, fmt.Errorf("decrypt spec is nil")
	}
	if len(spec.Data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}
	if len(spec.Key) == 0 {
		return nil, fmt.Errorf("key is empty")
	}

	switch spec.Algorithm {
	case "aes-cbc", "aes-ecb", "aes-cfb", "aes-ofb", "aes-ctr", "aes-gcm":
		return decryptLoopHTTPFuzzAES(spec)
	case "des-cbc", "des-ecb":
		return decryptLoopHTTPFuzzDES(spec)
	case "3des-cbc", "3des-ecb":
		return decryptLoopHTTPFuzzTripleDES(spec)
	case "rc4":
		return codec.RC4Decrypt(spec.Key, spec.Data)
	case "sm4-cbc", "sm4-ecb":
		return decryptLoopHTTPFuzzSM4(spec)
	case "rsa":
		return tlsutils.RSADecryptWithPKCS1v15Block(string(spec.Key), spec.Data)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", spec.Algorithm)
	}
}

// decryptLoopHTTPFuzzAES 处理 AES 全模式解密：块模式支持 pkcs7/zero 填充，流模式无需填充。
func decryptLoopHTTPFuzzAES(spec *loopHTTPFuzzDecryptSpec) ([]byte, error) {
	switch spec.Algorithm {
	case "aes-cbc":
		switch spec.Padding {
		case "zero":
			return codec.AESDecryptCBCWithZeroPadding(spec.Key, spec.Data, spec.IV)
		case "none":
			return codec.AESDec(spec.Key, spec.Data, spec.IV, codec.CBC)
		default: // pkcs7
			return codec.AESDecryptCBCWithPKCSPadding(spec.Key, spec.Data, spec.IV)
		}
	case "aes-ecb":
		switch spec.Padding {
		case "zero":
			return codec.AESDecryptECBWithZeroPadding(spec.Key, spec.Data, spec.IV)
		case "none":
			return codec.AESDec(spec.Key, spec.Data, nil, codec.ECB)
		default: // pkcs7
			return codec.AESDecryptECBWithPKCSPadding(spec.Key, spec.Data, spec.IV)
		}
	case "aes-cfb":
		switch spec.Padding {
		case "zero":
			return codec.AESDecryptCFBWithZeroPadding(spec.Key, spec.Data, spec.IV)
		case "none":
			return codec.AESDec(spec.Key, spec.Data, spec.IV, codec.CFB)
		default: // pkcs7
			return codec.AESDecryptCFBWithPKCSPadding(spec.Key, spec.Data, spec.IV)
		}
	case "aes-ofb":
		return codec.AESDec(spec.Key, spec.Data, spec.IV, codec.OFB)
	case "aes-ctr":
		return codec.AESDec(spec.Key, spec.Data, spec.IV, codec.CTR)
	case "aes-gcm":
		return codec.AESGCMDecrypt(spec.Key, spec.Data, spec.IV)
	default:
		return nil, fmt.Errorf("unsupported AES mode: %s", spec.Algorithm)
	}
}

// decryptLoopHTTPFuzzDES 处理 DES CBC/ECB 解密，按 padding 去填充。
func decryptLoopHTTPFuzzDES(spec *loopHTTPFuzzDecryptSpec) ([]byte, error) {
	var raw []byte
	var err error
	if spec.Algorithm == "des-ecb" {
		raw, err = codec.DESDec(spec.Key, spec.Data, nil, codec.ECB)
	} else {
		raw, err = codec.DESDec(spec.Key, spec.Data, spec.IV, codec.CBC)
	}
	if err != nil {
		return nil, err
	}
	return unpadLoopHTTPFuzzBlock(raw, 8, spec.Padding)
}

// decryptLoopHTTPFuzzTripleDES 处理 3DES CBC/ECB 解密，按 padding 去填充。
func decryptLoopHTTPFuzzTripleDES(spec *loopHTTPFuzzDecryptSpec) ([]byte, error) {
	var raw []byte
	var err error
	if spec.Algorithm == "3des-ecb" {
		raw, err = codec.TripleDesDec(spec.Key, spec.Data, nil, codec.ECB)
	} else {
		raw, err = codec.TripleDesDec(spec.Key, spec.Data, spec.IV, codec.CBC)
	}
	if err != nil {
		return nil, err
	}
	return unpadLoopHTTPFuzzBlock(raw, 8, spec.Padding)
}

// decryptLoopHTTPFuzzSM4 处理 SM4 CBC/ECB 解密，按 padding 去填充。
func decryptLoopHTTPFuzzSM4(spec *loopHTTPFuzzDecryptSpec) ([]byte, error) {
	var raw []byte
	var err error
	if spec.Algorithm == "sm4-ecb" {
		raw, err = codec.SM4Dec(spec.Key, spec.Data, nil, codec.ECB)
	} else {
		raw, err = codec.SM4Dec(spec.Key, spec.Data, spec.IV, codec.CBC)
	}
	if err != nil {
		return nil, err
	}
	return unpadLoopHTTPFuzzBlock(raw, 16, spec.Padding)
}

// unpadLoopHTTPFuzzBlock 按指定块大小和 padding 模式去填充。
func unpadLoopHTTPFuzzBlock(raw []byte, blockSize int, padding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(padding)) {
	case "pkcs7":
		if blockSize == 8 {
			return codec.PKCS7UnPaddingFor8ByteBlock(raw), nil
		}
		return codec.PKCS7UnPadding(raw), nil
	case "zero":
		return codec.ZeroUnPadding(raw), nil
	default: // none
		return raw, nil
	}
}

// encodeLoopHTTPFuzzDecryptOutput 按输出编码渲染明文；utf8 输出但内容非法时自动回退 hex。
func encodeLoopHTTPFuzzDecryptOutput(plain []byte, outputEncoding string) string {
	switch strings.ToLower(strings.TrimSpace(outputEncoding)) {
	case "hex":
		return codec.EncodeToHex(plain)
	case "base64":
		return codec.EncodeBase64(plain)
	default: // utf8
		if utf8.Valid(plain) {
			return string(plain)
		}
		return "(非 UTF-8 二进制，以下为 hex 表示)\n" + codec.EncodeToHex(plain)
	}
}

// buildLoopHTTPFuzzDecryptResultMarkdown 构造用户可读的解密结果 markdown。
func buildLoopHTTPFuzzDecryptResultMarkdown(spec *loopHTTPFuzzDecryptSpec, plain []byte, outputText string) string {
	var out strings.Builder
	out.WriteString("## 解密结果\n\n")
	out.WriteString(fmt.Sprintf("- 算法：`%s`\n", spec.Algorithm))
	out.WriteString(fmt.Sprintf("- 填充：`%s`\n", spec.Padding))
	out.WriteString(fmt.Sprintf("- 密钥编码：`%s`；IV 编码：`%s`；密文编码：`%s`\n", spec.KeyEncoding, spec.IVEncoding, spec.DataEncoding))
	out.WriteString(fmt.Sprintf("- 明文长度：%d 字节\n", len(plain)))
	if len(spec.IV) > 0 {
		out.WriteString(fmt.Sprintf("- IV：`%s`\n", codec.EncodeToHex(spec.IV)))
	}
	out.WriteString("\n### 明文\n\n```text\n")
	out.WriteString(outputText)
	out.WriteString("\n```\n")
	return out.String()
}

// looksLikeLoopHTTPFuzzNonFuzzDataTask 判断用户输入是否属于不需要 HTTP 请求的离线数据处理任务
// （解密/加密/编码/变换等）。用于 bootstrap 失败时决定是否继续循环而不是直接退出。
func looksLikeLoopHTTPFuzzNonFuzzDataTask(userInput string) bool {
	lower := strings.ToLower(strings.TrimSpace(userInput))
	if lower == "" {
		return false
	}
	// 如果输入里明显包含 URL 或 HTTP 报文特征，说明用户是想测 HTTP 目标，
	// 不应视为离线数据处理任务。
	if urlPattern.MatchString(userInput) {
		return false
	}
	if strings.Contains(lower, "http/1.1") || strings.Contains(lower, "http/1.0") || strings.Contains(lower, "get ") || strings.Contains(lower, "post ") {
		return false
	}
	signals := []string{
		"解密", "加密", "解密数据", "加密数据", "密文", "明文",
		"decrypt", "encrypt", "cipher", "ciphertext", "plaintext", "crypto",
		"密钥", "解码", "编码", "decode", "encode", "transform", "转换", "哈希", "hash", "摘要", "sign", "签名", "jwt",
	}
	for _, signal := range signals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	// 短 token（key/iv/aes/des/rc4/sm4/rsa/gcm/cbc/ecb）使用词边界匹配，避免把
	// hockey/monkey/keyboard 等普通词误判为离线数据处理任务。
	shortTokens := []string{"key", "iv", "aes", "des", "rc4", "sm4", "rsa", "gcm", "cbc", "ecb"}
	for _, token := range shortTokens {
		if regexpWordBoundaryMatch(lower, token) {
			return true
		}
	}
	return false
}

// regexpWordBoundaryMatch 用词边界正则匹配 token，避免子串误伤。
func regexpWordBoundaryMatch(text, token string) bool {
	matched, _ := regexp.MatchString(`(^|[^a-z0-9])`+regexp.QuoteMeta(token)+`($|[^a-z0-9])`, text)
	return matched
}

// buildLoopHTTPFuzzDecryptParamSummary 生成动作记录用的参数摘要。
func buildLoopHTTPFuzzDecryptParamSummary(spec *loopHTTPFuzzDecryptSpec) string {
	if spec == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("algorithm=%s", spec.Algorithm),
		fmt.Sprintf("padding=%s", spec.Padding),
		fmt.Sprintf("data_encoding=%s", spec.DataEncoding),
		fmt.Sprintf("key_encoding=%s", spec.KeyEncoding),
		fmt.Sprintf("iv_encoding=%s", spec.IVEncoding),
		fmt.Sprintf("output_encoding=%s", spec.OutputEncoding),
		fmt.Sprintf("key_len=%d", len(spec.Key)),
		fmt.Sprintf("data_len=%d", len(spec.Data)),
	}
	if len(spec.IV) > 0 {
		parts = append(parts, fmt.Sprintf("iv_len=%d", len(spec.IV)))
	}
	return strings.Join(parts, "; ")
}
