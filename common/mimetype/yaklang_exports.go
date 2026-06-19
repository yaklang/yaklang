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

// mimetype.DetectFile 读取指定文件并判断其具体 MIME 类型
// 通过读取文件头部的魔数（magic number）进行识别，不依赖文件扩展名
//
// 参数:
//   - i: 待检测的文件路径
//
// 返回值:
//   - MIME 检测结果对象，可调用 String() 获取形如 "text/plain; charset=utf-8" 的字符串
//   - 错误信息（文件不存在、为目录或读取失败时返回）
//
// Example:
// ```
// fp = file.Join(os.TempDir(), "yak_mime_demo.txt")
// file.Save(fp, "hello yak")~
// defer file.Remove(fp)
// mime = mimetype.DetectFile(fp)~
// println(mime.String())
// assert mime.String().Contains("text/plain"), "text file should be detected as text/plain"
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

// mimetype.Detect 判断一段数据的具体 MIME 类型，支持 io.Reader、[]byte 与 string 输入
// 通过数据头部的魔数（magic number）进行识别；传入 io.Reader 时最多读取 5 秒
//
// 参数:
//   - i: 待检测的数据，可为 string、[]byte 或 io.Reader
//
// 返回值:
//   - MIME 检测结果对象，可调用 String() 获取形如 "text/plain; charset=utf-8" 的字符串
//   - 错误信息（从 io.Reader 读取失败时返回）
//
// Example:
// ```
// mime = mimetype.Detect("hello yak")~
// println(mime.String())
// assert mime != nil, "Detect should return a MIME instance"
// assert mime.String().Contains("text/plain"), "plain ascii text should be detected as text/plain"
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
