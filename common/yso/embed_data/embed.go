package embeddata

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

const ysoXorKey = "yaklang-yso-v1"

//go:embed check_list.json.enc
//go:embed serialized_objects.json.enc
var ysoEncFS embed.FS

type SerializedObjects struct {
	ObjectArray     string `json:"object_array"`
	DirtyDataHeader string `json:"dirty_data_header"`
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
		content, err := xorencoded.LoadEmbedFile(ysoEncFS, "check_list.json.enc", []byte(ysoXorKey))
		if err != nil {
			panic(fmt.Sprintf("load check_list failed: %v", err))
		}
		if err := json.Unmarshal([]byte(content), &checkListCache); err != nil {
			panic(fmt.Sprintf("unmarshal check_list failed: %v", err))
		}
	})
	return checkListCache
}

// LoadSerializedObjects 从 XOR 编码的 embed 文件加载序列化对象 base64 字符串。
func LoadSerializedObjects() *SerializedObjects {
	serializedOnce.Do(func() {
		content, err := xorencoded.LoadEmbedFile(ysoEncFS, "serialized_objects.json.enc", []byte(ysoXorKey))
		if err != nil {
			panic(fmt.Sprintf("load serialized_objects failed: %v", err))
		}
		s := &SerializedObjects{}
		if err := json.Unmarshal([]byte(content), s); err != nil {
			panic(fmt.Sprintf("unmarshal serialized_objects failed: %v", err))
		}
		serializedCache = s
	})
	return serializedCache
}
