package embeddata

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

const yserxXorKey = "yaklang-yserx-v1"

//go:embed serialized_objects.json.enc
var yserxEncFS embed.FS

type SerializedObjects struct {
	ObjectArray         string `json:"object_array"`
	DirtyDataHeader     string `json:"dirty_data_header"`
	ByteArraySerialUID  string `json:"byte_array_serial_uid"`
}

var (
	cache *SerializedObjects
	once  sync.Once
)

func LoadSerializedObjects() *SerializedObjects {
	once.Do(func() {
		content, err := xorencoded.LoadEmbedFile(yserxEncFS, "serialized_objects.json.enc", []byte(yserxXorKey))
		if err != nil {
			panic(fmt.Sprintf("load serialized_objects failed: %v", err))
		}
		s := &SerializedObjects{}
		if err := json.Unmarshal([]byte(content), s); err != nil {
			panic(fmt.Sprintf("unmarshal serialized_objects failed: %v", err))
		}
		cache = s
	})
	return cache
}
