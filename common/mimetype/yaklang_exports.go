package mimetype

import (
	"context"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"time"
)

// mimetype.DetectFile 判断一个文件的具体MIME
// Example:
// ```
// result = mimetype.DetectFile("path/to/file")~ // with wavycall
// result2, err = mimetype.DetectFile("path/to/file2") // with err return
// ```
func _mimetypeDetectFile(i string) (*MIME, error) {
	stat, err := os.Stat(i)
	if os.IsNotExist(err) {
		return nil, err
	}
	if stat.IsDir() {
		return nil, utils.Errorf("%v is a directory, not a file", i)
	}
	return DetectFile(i)
}

// mimetype.Detect 判断一个数据的具体MIME，支持 io.Reader 输入和 []byte/string 输入
// Example:
// ```
// result = mimetype.Detect("hello yak")~ // with wavycall
// ```
func _mimetypeDetect(i any) (*MIME, error) {
	// check if io.Reader
	switch ret := i.(type) {
	case io.Reader:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		results, err := io.ReadAll(ctxio.NewReader(ctx, ret))
		if len(results) > 0 {
			return _mimetypeDetect(results)
		}
		return nil, utils.Errorf("cannot fetch data from io.Reader, reason: %v", err)
	default:
		return Detect(utils.InterfaceToBytes(i)), nil
	}
}

var Exports = map[string]any{}
