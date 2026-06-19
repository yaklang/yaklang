package ziputil

import (
	"io"
	"io/ioutil"
	"runtime"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memfile"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

type ExtractResult struct {
	FileName string
	Content  []byte
	Error    error
}

// ExtractFile 从 ZIP 文件中提取单个文件的内容
// 参数:
//   - zipFile: zip 文件路径
//   - targetFile: 待提取的 zip 内条目名称
//
// 返回值:
//   - 提取出的文件内容字节
//   - 错误信息
//
// Example:
// ```
// content = zip.ExtractFile("/tmp/abc.zip", "a.txt")~
// ```
func ExtractFile(zipFile string, targetFile string) ([]byte, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return ExtractFileFromRaw(raw, targetFile)
}

// ExtractFileFromRaw 从内存中的 ZIP 原始数据中提取单个文件的内容
// 参数:
//   - raw: zip 的原始数据（[]byte、string 或 io.Reader）
//   - targetFile: 待提取的 zip 内条目名称
//
// 返回值:
//   - 提取出的文件内容字节
//   - 错误信息
//
// Example:
// ```
// zipBytes = zip.CompressRaw({"a.txt": "hello world"})~
// content = zip.ExtractFileFromRaw(zipBytes, "a.txt")~
// assert string(content) == "hello world", "ExtractFileFromRaw should return the entry content"
// ```
func ExtractFileFromRaw(raw interface{}, targetFile string) ([]byte, error) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case io.Reader:
		var err error
		data, err = io.ReadAll(v)
		if err != nil {
			return nil, utils.Errorf("read data from reader failed: %s", err)
		}
	default:
		return nil, utils.Error("unsupported raw type, must be []byte, string or io.Reader")
	}

	size := len(data)
	mfile := memfile.New(data)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return nil, utils.Errorf("create zip reader failed: %s", err)
	}

	for _, file := range reader.File {
		if file.Name == targetFile {
			rc, err := file.Open()
			if err != nil {
				return nil, utils.Errorf("open file %s in zip failed: %s", targetFile, err)
			}
			defer rc.Close()

			content, err := io.ReadAll(rc)
			if err != nil {
				return nil, utils.Errorf("read file %s content failed: %s", targetFile, err)
			}
			return content, nil
		}
	}

	return nil, utils.Errorf("file %s not found in zip", targetFile)
}

// ExtractFiles 从 ZIP 文件中并发提取多个文件
// 参数:
//   - zipFile: zip 文件路径
//   - targetFiles: 待提取的条目名称列表
//
// 返回值:
//   - 提取结果列表（每项含 FileName/Content/Error）
//   - 错误信息
//
// Example:
// ```
// results = zip.ExtractFiles("/tmp/abc.zip", ["a.txt", "b.txt"])~
// for r in results { println(r.FileName) }
// ```
func ExtractFiles(zipFile string, targetFiles []string) ([]*ExtractResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return ExtractFilesFromRaw(raw, targetFiles)
}

// ExtractFilesFromRaw 从内存中的 ZIP 原始数据中并发提取多个文件
// 参数:
//   - raw: zip 的原始数据（[]byte、string 或 io.Reader）
//   - targetFiles: 待提取的条目名称列表
//
// 返回值:
//   - 提取结果列表（每项含 FileName/Content/Error）
//   - 错误信息
//
// Example:
// ```
// zipBytes = zip.CompressRaw({"a.txt": "AAA", "b.txt": "BBB"})~
// results = zip.ExtractFilesFromRaw(zipBytes, ["a.txt"])~
// assert len(results) == 1, "ExtractFilesFromRaw should return one matched file"
// assert string(results[0].Content) == "AAA", "extracted content should match"
// ```
func ExtractFilesFromRaw(raw interface{}, targetFiles []string) ([]*ExtractResult, error) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case io.Reader:
		var err error
		data, err = io.ReadAll(v)
		if err != nil {
			return nil, utils.Errorf("read data from reader failed: %s", err)
		}
	default:
		return nil, utils.Error("unsupported raw type, must be []byte, string or io.Reader")
	}

	size := len(data)
	mfile := memfile.New(data)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return nil, utils.Errorf("create zip reader failed: %s", err)
	}

	// 构建目标文件映射
	targetMap := make(map[string]bool)
	for _, target := range targetFiles {
		targetMap[target] = true
	}

	// 计算并发数
	concurrency := runtime.NumCPU()
	if concurrency > 8 {
		concurrency = 8
	}
	semaphore := make(chan struct{}, concurrency)

	var (
		results   []*ExtractResult
		resultsMu sync.Mutex
		wg        sync.WaitGroup
	)

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// 检查是否是目标文件
		if !targetMap[file.Name] {
			continue
		}

		wg.Add(1)
		go func(f *zip.File) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &ExtractResult{
				FileName: f.Name,
			}

			rc, err := f.Open()
			if err != nil {
				result.Error = utils.Errorf("open file %s in zip failed: %s", f.Name, err)
				log.Errorf("extract file %s failed: %s", f.Name, err)
			} else {
				defer rc.Close()
				content, err := io.ReadAll(rc)
				if err != nil {
					result.Error = utils.Errorf("read file %s content failed: %s", f.Name, err)
					log.Errorf("read file %s failed: %s", f.Name, err)
				} else {
					result.Content = content
				}
			}

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()
		}(file)
	}

	wg.Wait()

	return results, nil
}

// ExtractByPattern 根据文件名模式（支持 * 通配符）从 ZIP 文件中提取匹配的文件
// 参数:
//   - zipFile: zip 文件路径
//   - pattern: 文件名匹配模式（如 "*.txt"）
//
// 返回值:
//   - 提取结果列表（每项含 FileName/Content/Error）
//   - 错误信息
//
// Example:
// ```
// results = zip.ExtractByPattern("/tmp/abc.zip", "*.txt")~
// for r in results { println(r.FileName) }
// ```
func ExtractByPattern(zipFile string, pattern string) ([]*ExtractResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return ExtractByPatternFromRaw(raw, pattern)
}

// ExtractByPatternFromRaw 从内存中的 ZIP 原始数据按文件名模式（支持 * 通配符）提取匹配的文件
// 参数:
//   - raw: zip 的原始数据（[]byte、string 或 io.Reader）
//   - pattern: 文件名匹配模式（如 "*.txt"）
//
// 返回值:
//   - 提取结果列表（每项含 FileName/Content/Error）
//   - 错误信息
//
// Example:
// ```
// zipBytes = zip.CompressRaw({"a.txt": "AAA", "b.log": "BBB"})~
// results = zip.ExtractByPatternFromRaw(zipBytes, "*.txt")~
// assert len(results) == 1, "only the .txt entry should match the pattern"
// ```
func ExtractByPatternFromRaw(raw interface{}, pattern string) ([]*ExtractResult, error) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case io.Reader:
		var err error
		data, err = io.ReadAll(v)
		if err != nil {
			return nil, utils.Errorf("read data from reader failed: %s", err)
		}
	default:
		return nil, utils.Error("unsupported raw type, must be []byte, string or io.Reader")
	}

	size := len(data)
	mfile := memfile.New(data)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return nil, utils.Errorf("create zip reader failed: %s", err)
	}

	// 计算并发数
	concurrency := runtime.NumCPU()
	if concurrency > 8 {
		concurrency = 8
	}
	semaphore := make(chan struct{}, concurrency)

	var (
		results   []*ExtractResult
		resultsMu sync.Mutex
		wg        sync.WaitGroup
	)

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// 简单通配符匹配
		matched := matchPattern(file.Name, pattern)
		if !matched {
			continue
		}

		wg.Add(1)
		go func(f *zip.File) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &ExtractResult{
				FileName: f.Name,
			}

			rc, err := f.Open()
			if err != nil {
				result.Error = utils.Errorf("open file %s in zip failed: %s", f.Name, err)
				log.Errorf("extract file %s failed: %s", f.Name, err)
			} else {
				defer rc.Close()
				content, err := io.ReadAll(rc)
				if err != nil {
					result.Error = utils.Errorf("read file %s content failed: %s", f.Name, err)
					log.Errorf("read file %s failed: %s", f.Name, err)
				} else {
					result.Content = content
				}
			}

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()
		}(file)
	}

	wg.Wait()

	return results, nil
}

// matchPattern 简单的通配符匹配
func matchPattern(name, pattern string) bool {
	// 支持 * 和 ? 通配符
	if pattern == "*" {
		return true
	}

	// 如果包含 *，进行简单匹配
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 0 {
			return true
		}

		// 检查第一部分
		if parts[0] != "" && !strings.HasPrefix(name, parts[0]) {
			return false
		}

		// 检查最后一部分
		if parts[len(parts)-1] != "" && !strings.HasSuffix(name, parts[len(parts)-1]) {
			return false
		}

		// 检查中间部分
		pos := 0
		for i, part := range parts {
			if i == 0 || i == len(parts)-1 || part == "" {
				if i == 0 && part != "" {
					pos = len(part)
				}
				continue
			}
			idx := strings.Index(name[pos:], part)
			if idx < 0 {
				return false
			}
			pos += idx + len(part)
		}

		return true
	}

	// 不包含通配符，直接比较
	return name == pattern
}
