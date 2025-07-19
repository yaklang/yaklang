package mimetype

import (
	"context"
	jsonlib "encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"io"
	"os"
	"strconv"
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
		return nil, errors.Errorf("%v is a directory, not a file", i)
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
		return nil, errors.Errorf("cannot fetch data from io.Reader, reason: %v", err)
	default:
		return Detect(AnyToBytes(i)), nil
	}
}

func AnyToBytes(i interface{}) (result []byte) {
	var b []byte
	defer func() {
		if err := recover(); err != nil {
			result = []byte(fmt.Sprintf("%v", i))
		}
	}()

	if i == nil {
		return []byte{}
	}

	switch s := i.(type) {
	case nil:
		return []byte{}
	case string:
		b = []byte(s)
	case []byte:
		b = s[0:]
	case bool:
		b = []byte(strconv.FormatBool(s))
	case float64:
		return []byte(strconv.FormatFloat(s, 'f', -1, 64))
	case float32:
		return []byte(strconv.FormatFloat(float64(s), 'f', -1, 32))
	case int:
		return []byte(strconv.Itoa(s))
	case int64:
		return []byte(strconv.FormatInt(s, 10))
	case int32:
		return []byte(strconv.Itoa(int(s)))
	case int16:
		return []byte(strconv.FormatInt(int64(s), 10))
	case int8:
		return []byte(strconv.FormatInt(int64(s), 10))
	case uint:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case uint64:
		return []byte(strconv.FormatUint(s, 10))
	case uint32:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case uint16:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case uint8:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case fmt.Stringer:
		return []byte(s.String())
	case error:
		return []byte(s.Error())
	// case io.Reader:
	//	if ret != nil && ret.Read != nil {
	//		bytes, _ = ioutil.ReadAll(ret)
	//		return bytes
	//	}
	//	return []byte(fmt.Sprintf("%v", i))
	default:
		// 尝试将i作为map转换成JSON
		if jsonBytes, err := jsonlib.Marshal(i); err == nil {
			b = jsonBytes
		} else {
			// 如果转换失败，则回退到使用fmt.Sprintf
			b = []byte(fmt.Sprintf("%v", i))
		}
	}

	return b
}

var Exports = map[string]any{
	"Detect":     _mimetypeDetect,
	"DetectFile": _mimetypeDetectFile,
}
