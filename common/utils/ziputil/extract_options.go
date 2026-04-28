package ziputil

import (
	"io"
	"io/ioutil"
	"runtime"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memfile"
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

// 带密码的 zip 提取入口
// 关键词: zip 文件提取, 密码 zip 提取

// ExtractFileWithOptions 提取单个文件，支持 ExtractOption（含密码）
// 关键词: ExtractFile, 密码提取
func ExtractFileWithOptions(zipFile, targetFile string, opts ...ExtractOption) ([]byte, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return ExtractFileFromRawWithOptions(raw, targetFile, opts...)
}

// ExtractFileFromRawWithOptions 从原始字节提取单个文件
// 关键词: ExtractFileFromRaw, 内存提取, 密码提取
func ExtractFileFromRawWithOptions(raw interface{}, targetFile string, opts ...ExtractOption) ([]byte, error) {
	cfg := newExtractConfig(opts...)
	data, err := normalizeZipRaw(raw)
	if err != nil {
		return nil, err
	}

	size := len(data)
	mfile := memfile.New(data)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return nil, utils.Errorf("create zip reader failed: %s", err)
	}

	for _, file := range reader.File {
		if file.Name != targetFile {
			continue
		}
		if file.IsEncrypted() {
			if cfg.Password == "" {
				return nil, utils.Errorf("file %s is encrypted but no password supplied", file.Name)
			}
			file.SetPassword(cfg.Password)
		}
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

	return nil, utils.Errorf("file %s not found in zip", targetFile)
}

// ExtractFilesWithOptions 并发提取多个文件，支持 ExtractOption（含密码）
// 关键词: ExtractFiles, 并发提取, 密码提取
func ExtractFilesWithOptions(zipFile string, targetFiles []string, opts ...ExtractOption) ([]*ExtractResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return ExtractFilesFromRawWithOptions(raw, targetFiles, opts...)
}

// ExtractFilesFromRawWithOptions 从原始字节并发提取多个文件
// 关键词: ExtractFilesFromRaw, 并发提取, 密码提取
func ExtractFilesFromRawWithOptions(raw interface{}, targetFiles []string, opts ...ExtractOption) ([]*ExtractResult, error) {
	cfg := newExtractConfig(opts...)
	data, err := normalizeZipRaw(raw)
	if err != nil {
		return nil, err
	}

	size := len(data)
	mfile := memfile.New(data)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return nil, utils.Errorf("create zip reader failed: %s", err)
	}

	targetMap := make(map[string]bool)
	for _, target := range targetFiles {
		targetMap[target] = true
	}

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
		if !targetMap[file.Name] {
			continue
		}

		wg.Add(1)
		go func(f *zip.File) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &ExtractResult{FileName: f.Name}

			if f.IsEncrypted() {
				if cfg.Password == "" {
					result.Error = utils.Errorf("file %s is encrypted but no password supplied", f.Name)
					resultsMu.Lock()
					results = append(results, result)
					resultsMu.Unlock()
					return
				}
				f.SetPassword(cfg.Password)
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

// ExtractByPatternWithOptions 根据通配符提取多个文件
// 关键词: ExtractByPattern, 通配符提取, 密码提取
func ExtractByPatternWithOptions(zipFile string, pattern string, opts ...ExtractOption) ([]*ExtractResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return ExtractByPatternFromRawWithOptions(raw, pattern, opts...)
}

// ExtractByPatternFromRawWithOptions 从原始字节按通配符提取
// 关键词: ExtractByPatternFromRaw, 内存通配符提取, 密码提取
func ExtractByPatternFromRawWithOptions(raw interface{}, pattern string, opts ...ExtractOption) ([]*ExtractResult, error) {
	cfg := newExtractConfig(opts...)
	data, err := normalizeZipRaw(raw)
	if err != nil {
		return nil, err
	}

	size := len(data)
	mfile := memfile.New(data)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return nil, utils.Errorf("create zip reader failed: %s", err)
	}

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
		if !matchPattern(file.Name, pattern) {
			continue
		}

		wg.Add(1)
		go func(f *zip.File) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &ExtractResult{FileName: f.Name}

			if f.IsEncrypted() {
				if cfg.Password == "" {
					result.Error = utils.Errorf("file %s is encrypted but no password supplied", f.Name)
					resultsMu.Lock()
					results = append(results, result)
					resultsMu.Unlock()
					return
				}
				f.SetPassword(cfg.Password)
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

// normalizeZipRaw 把多种类型的原始数据归一化为 []byte
// 关键词: zip 字节归一化
func normalizeZipRaw(raw interface{}) ([]byte, error) {
	switch v := raw.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case io.Reader:
		data, err := io.ReadAll(v)
		if err != nil {
			return nil, utils.Errorf("read data from reader failed: %s", err)
		}
		return data, nil
	default:
		return nil, utils.Error("unsupported raw type, must be []byte, string or io.Reader")
	}
}
