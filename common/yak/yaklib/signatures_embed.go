//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-sigs-v1 --no-embed
package yaklib

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"encoding/json"
	"fmt"
)

const sigsXorKey = "yaklang-sigs-v1"

//go:embed static
var sigsRawFS embed.FS

var sigsFS = filesys.NewEmbedSubFS(sigsRawFS, "static")

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
