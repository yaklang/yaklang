//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-yserx-v1 --no-embed
package embeddata

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

const yserxXorKey = "yaklang-yserx-v1"

//go:embed static.tar.gz
var yserxEncFS embed.FS

var yserxFS = func() *gzip_embed.PreprocessingEmbed {
	ins, err := gzip_embed.NewPreprocessingEmbedWithXORKey(&yserxEncFS, "static.tar.gz", true, []byte(yserxXorKey))
	if err != nil {
		panic(fmt.Sprintf("init yserx embed failed: %v", err))
	}
	return ins
}()

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
