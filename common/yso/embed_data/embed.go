//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-yso-v1 --no-embed
package embeddata

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

const ysoXorKey = "yaklang-yso-v1"

//go:embed static.tar.gz
var ysoEncFS embed.FS

var ysoFS = func() *gzip_embed.PreprocessingEmbed {
	ins, err := gzip_embed.NewPreprocessingEmbedWithXORKey(&ysoEncFS, "static.tar.gz", true, []byte(ysoXorKey))
	if err != nil {
		panic(fmt.Sprintf("init yso embed failed: %v", err))
	}
	return ins
}()

type SerializedObjects struct {
	ObjectArray        string `json:"object_array"`
	DirtyDataHeader    string `json:"dirty_data_header"`
}

var (
	checkListCache    map[string]string
	serializedCache   *SerializedObjects
	checkListOnce     sync.Once
	serializedOnce    sync.Once
)

// LoadCheckList 从 XOR 编码的 embed 文件加载 gadget 检测类名列表。
func LoadCheckList() map[string]string {
	checkListOnce.Do(func() {
		raw, err := ysoFS.ReadFile("check_list.json")
		if err != nil {
			panic(fmt.Sprintf("read check_list.json failed: %v", err))
		}
		if err := json.Unmarshal(raw, &checkListCache); err != nil {
			panic(fmt.Sprintf("unmarshal check_list failed: %v", err))
		}
	})
	return checkListCache
}

// LoadSerializedObjects 从 XOR 编码的 embed 文件加载序列化对象 base64 字符串。
func LoadSerializedObjects() *SerializedObjects {
	serializedOnce.Do(func() {
		raw, err := ysoFS.ReadFile("serialized_objects.json")
		if err != nil {
			panic(fmt.Sprintf("read serialized_objects.json failed: %v", err))
		}
		s := &SerializedObjects{}
		if err := json.Unmarshal(raw, s); err != nil {
			panic(fmt.Sprintf("unmarshal serialized_objects failed: %v", err))
		}
		serializedCache = s
	})
	return serializedCache
}
