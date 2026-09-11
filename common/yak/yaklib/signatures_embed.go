//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-sigs-v1 --no-embed
package yaklib

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

const sigsXorKey = "yaklang-sigs-v1"

//go:embed static.tar.gz
var sigsEncFS embed.FS

var sigsFS = func() *gzip_embed.PreprocessingEmbed {
	ins, err := gzip_embed.NewPreprocessingEmbedWithXORKey(&sigsEncFS, "static.tar.gz", true, []byte(sigsXorKey))
	if err != nil {
		panic(err)
	}
	return ins
}()

// loadSignaturesFromEmbed 从 XOR 编码的 embed 文件加载恶意文件特征库。
func loadSignaturesFromEmbed() ([]*MaliciousSignature, error) {
	raw, err := sigsFS.ReadFile("malicious_signatures.json")
	if err != nil {
		return nil, fmt.Errorf("read malicious_signatures.json failed: %v", err)
	}
	var sigs []*MaliciousSignature
	if err := json.Unmarshal(raw, &sigs); err != nil {
		return nil, fmt.Errorf("unmarshal signatures failed: %v", err)
	}
	return sigs, nil
}
