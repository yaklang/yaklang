package binx

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"reflect"
)

func BinaryRead(data any, descriptors ...*PartDescriptor) ([]ResultIf, error) {
	var reader io.Reader
	switch ret := data.(type) {
	case io.Reader:
		reader = ret
	case []byte:
		reader = bytes.NewBuffer(ret)
	case string:
		reader = bytes.NewBufferString(ret)
	case []rune:
		reader = bytes.NewBufferString(string(ret))
	default:
		return nil, utils.Errorf("unexpected type for input: %v", reflect.TypeOf(ret))
	}
	var results []ResultIf
	var ctx = make([]ResultIf, 0)
	var ret []ResultIf
	var err error
	for _, des := range descriptors {
		ret, _, ctx, err = read(ctx, des, reader, 0)
		if err != nil {
			return results, err
		}
		results = append(results, ret...)
	}
	return results, nil
}
