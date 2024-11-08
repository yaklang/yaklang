package yakgrpc

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var reflectTypeCache = utils.NewTTLCache[reflect.Type](3 * time.Minute)

func marshalValue(v any) string {
	if bytes, err := json.Marshal(v); err == nil {
		return string(bytes)
	} else {
		return codec.AnyToString(v)
	}
}

func unmarshalValue(v string) any {
	if v == "" {
		return nil
	}
	var val any
	if err := json.Unmarshal([]byte(v), &val); err == nil {
		return val
	} else {
		return v
	}
}
