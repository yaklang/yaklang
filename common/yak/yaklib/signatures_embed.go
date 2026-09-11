package yaklib

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

const sigsXorKey = "yaklang-sigs-v1"

//go:embed embed_data/malicious_signatures.json.enc
var sigsEncFS embed.FS

// loadSignaturesFromEmbed 从 XOR 编码的 embed 文件加载恶意文件特征库。
func loadSignaturesFromEmbed() ([]*MaliciousSignature, error) {
	content, err := xorencoded.LoadEmbedFile(sigsEncFS, "embed_data/malicious_signatures.json.enc", []byte(sigsXorKey))
	if err != nil {
		return nil, err
	}
	var sigs []*MaliciousSignature
	if err := json.Unmarshal([]byte(content), &sigs); err != nil {
		return nil, fmt.Errorf("unmarshal signatures failed: %v", err)
	}
	return sigs, nil
}
