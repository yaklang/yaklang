//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-yserx-v1 --no-embed
package embeddata

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"encoding/json"
	"fmt"
	"sync"
)

const yserxXorKey = "yaklang-yserx-v1"

//go:embed static
var yserxRawFS embed.FS

var yserxFS = filesys.NewEmbedSubFS(yserxRawFS, "static")

type SerializedObjects struct {
	ObjectArray        string `json:"object_array"`
	DirtyDataHeader    string `json:"dirty_data_header"`
	ByteArraySerialUID string `json:"byte_array_serial_uid"`
}

var (
	cache *SerializedObjects
	once  sync.Once
)

func LoadSerializedObjects() *SerializedObjects {
	once.Do(func() {
		raw, err := yserxFS.ReadFile("serialized_objects.json")
		if err != nil {
			panic(fmt.Sprintf("read serialized_objects.json failed: %v", err))
		}
		s := &SerializedObjects{}
		if err := json.Unmarshal(raw, s); err != nil {
			panic(fmt.Sprintf("unmarshal serialized_objects failed: %v", err))
		}
		cache = s
	})
	return cache
}
