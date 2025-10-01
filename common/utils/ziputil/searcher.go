package ziputil

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

// ZipGrepSearcher 是一个带缓存的 ZIP 文件搜索器
// 可以缓存 ZIP 文件内容以加速多次搜索
type ZipGrepSearcher struct {
	zipFile   string
	rawData   []byte
	reader    *zip.Reader
	fileCache map[string]*fileContent // 文件内容缓存
	cacheMu   sync.RWMutex
	cacheAll  bool // 是否缓存所有文件内容
}

// fileContent 缓存的文件内容
type fileContent struct {
	name    string
	lines   []string
	content string
}

// NewZipGrepSearcher 创建一个新的 ZIP 搜索器（从文件）
func NewZipGrepSearcher(zipFile string) (*ZipGrepSearcher, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}

	return NewZipGrepSearcherFromRaw(raw, zipFile)
}

// NewZipGrepSearcherFromRaw 创建一个新的 ZIP 搜索器（从原始数据）
func NewZipGrepSearcherFromRaw(raw interface{}, filename ...string) (*ZipGrepSearcher, error) {
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

	zipFileName := "memory.zip"
	if len(filename) > 0 && filename[0] != "" {
		zipFileName = filename[0]
	}

	return &ZipGrepSearcher{
		zipFile:   zipFileName,
		rawData:   data,
		reader:    reader,
		fileCache: make(map[string]*fileContent),
		cacheAll:  false,
	}, nil
}

// WithCacheAll 设置是否预加载并缓存所有文件内容
func (s *ZipGrepSearcher) WithCacheAll(cacheAll bool) *ZipGrepSearcher {
	s.cacheAll = cacheAll
	if cacheAll {
		s.preloadAllFiles()
	}
	return s
}

// preloadAllFiles 预加载所有文件内容
func (s *ZipGrepSearcher) preloadAllFiles() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	for _, file := range s.reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// 如果已经缓存，跳过
		if _, exists := s.fileCache[file.Name]; exists {
			continue
		}

		// 读取文件内容
		rc, err := file.Open()
		if err != nil {
			log.Errorf("open file %s in zip failed: %s", file.Name, err)
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Errorf("read file %s content failed: %s", file.Name, err)
			continue
		}

		// 缓存内容
		lines := strings.Split(string(content), "\n")
		s.fileCache[file.Name] = &fileContent{
			name:    file.Name,
			lines:   lines,
			content: string(content),
		}
	}

	log.Infof("preloaded %d files from %s", len(s.fileCache), s.zipFile)
}

// getFileContent 获取文件内容（使用缓存）
func (s *ZipGrepSearcher) getFileContent(fileName string) (*fileContent, error) {
	// 先尝试从缓存读取
	s.cacheMu.RLock()
	cached, exists := s.fileCache[fileName]
	s.cacheMu.RUnlock()

	if exists {
		return cached, nil
	}

	// 缓存未命中，读取文件
	var targetFile *zip.File
	for _, file := range s.reader.File {
		if file.Name == fileName {
			targetFile = file
			break
		}
	}

	if targetFile == nil {
		return nil, utils.Errorf("file %s not found in zip", fileName)
	}

	rc, err := targetFile.Open()
	if err != nil {
		return nil, utils.Errorf("open file %s failed: %s", fileName, err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, utils.Errorf("read file %s content failed: %s", fileName, err)
	}

	// 创建并缓存
	lines := strings.Split(string(content), "\n")
	fc := &fileContent{
		name:    fileName,
		lines:   lines,
		content: string(content),
	}

	s.cacheMu.Lock()
	s.fileCache[fileName] = fc
	s.cacheMu.Unlock()

	return fc, nil
}

// GrepRegexp 使用正则表达式搜索
func (s *ZipGrepSearcher) GrepRegexp(pattern string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: true,
	}
	for _, opt := range opts {
		opt(config)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, utils.Errorf("compile regexp failed: %s", err)
	}

	return s.grepFiles(func(line string) bool {
		return re.MatchString(line)
	}, config, "regexp:"+pattern)
}

// GrepSubString 使用子字符串搜索
func (s *ZipGrepSearcher) GrepSubString(substring string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: false,
	}
	for _, opt := range opts {
		opt(config)
	}

	matcher := func(line string) bool {
		if config.CaseSensitive {
			return strings.Contains(line, substring)
		}
		return strings.Contains(strings.ToLower(line), strings.ToLower(substring))
	}

	return s.grepFiles(matcher, config, "substring:"+substring)
}

// GrepPathRegexp 使用正则表达式搜索文件路径
func (s *ZipGrepSearcher) GrepPathRegexp(pattern string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: true,
	}
	for _, opt := range opts {
		opt(config)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, utils.Errorf("compile regexp failed: %s", err)
	}

	return s.grepPaths(func(path string) bool {
		return re.MatchString(path)
	}, config, "path_regexp:"+pattern)
}

// GrepPathSubString 使用子字符串搜索文件路径
func (s *ZipGrepSearcher) GrepPathSubString(substring string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: false,
	}
	for _, opt := range opts {
		opt(config)
	}

	matcher := func(path string) bool {
		if config.CaseSensitive {
			return strings.Contains(path, substring)
		}
		return strings.Contains(strings.ToLower(path), strings.ToLower(substring))
	}

	return s.grepPaths(matcher, config, "path_substring:"+substring)
}

// grepPaths 在文件路径中搜索
func (s *ZipGrepSearcher) grepPaths(matcher func(string) bool, config *GrepConfig, method string) ([]*GrepResult, error) {
	var results []*GrepResult
	fileIndex := 0

	for _, file := range s.reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		fileIndex++

		// 应用路径过滤
		if !shouldIncludePath(file.Name, config) {
			continue
		}

		// 检查路径是否匹配
		if matcher(file.Name) {
			result := &GrepResult{
				FileName:    file.Name,
				LineNumber:  0,
				Line:        file.Name,
				ScoreMethod: method,
				Score:       1.0 / float64(fileIndex+1),
			}

			results = append(results, result)

			// 检查是否达到限制
			if config.Limit > 0 && len(results) >= config.Limit {
				break
			}
		}
	}

	return results, nil
}

// grepFiles 在所有文件中搜索
func (s *ZipGrepSearcher) grepFiles(matcher func(string) bool, config *GrepConfig, method string) ([]*GrepResult, error) {
	var results []*GrepResult
	var resultsMu sync.Mutex

	// 遍历所有文件
	for _, file := range s.reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// 应用路径过滤
		if !shouldIncludePath(file.Name, config) {
			continue
		}

		// 获取文件内容
		fc, err := s.getFileContent(file.Name)
		if err != nil {
			log.Errorf("get file content failed: %s", err)
			continue
		}

		// 在文件中搜索
		fileResults := s.grepFileContent(fc, matcher, config, method)
		if len(fileResults) > 0 {
			resultsMu.Lock()
			results = append(results, fileResults...)
			resultsMu.Unlock()

			// 检查是否达到限制
			if config.Limit > 0 && len(results) >= config.Limit {
				break
			}
		}
	}

	// 应用限制
	if config.Limit > 0 && len(results) > config.Limit {
		results = results[:config.Limit]
	}

	return results, nil
}

// grepFileContent 在单个文件内容中搜索
func (s *ZipGrepSearcher) grepFileContent(fc *fileContent, matcher func(string) bool, config *GrepConfig, method string) []*GrepResult {
	var results []*GrepResult
	contextBuffer := make([]string, 0, config.Context*2+1)

	for lineNumber, line := range fc.lines {
		actualLineNumber := lineNumber + 1 // 行号从 1 开始

		// 维护上下文缓冲区
		if config.Context > 0 {
			contextBuffer = append(contextBuffer, line)
			if len(contextBuffer) > config.Context*2+1 {
				contextBuffer = contextBuffer[1:]
			}
		}

		if matcher(line) {
			result := &GrepResult{
				FileName:    fc.name,
				LineNumber:  actualLineNumber,
				Line:        line,
				ScoreMethod: method,
				Score:       1.0 / float64(actualLineNumber+1), // 默认得分
			}

			// 添加上下文
			if config.Context > 0 {
				// 前置上下文
				contextStart := len(contextBuffer) - config.Context - 1
				if contextStart < 0 {
					contextStart = 0
				}
				contextEnd := len(contextBuffer) - 1
				if contextEnd >= 0 && contextStart < contextEnd {
					result.ContextBefore = make([]string, contextEnd-contextStart)
					copy(result.ContextBefore, contextBuffer[contextStart:contextEnd])
				}

				// 后置上下文
				afterLines := []string{}
				for i := 1; i <= config.Context && lineNumber+i < len(fc.lines); i++ {
					afterLines = append(afterLines, fc.lines[lineNumber+i])
				}
				result.ContextAfter = afterLines
			}

			results = append(results, result)

			// 检查是否达到限制
			if config.Limit > 0 && len(results) >= config.Limit {
				break
			}
		}
	}

	return results
}

// GrepRegexpInFile 在指定文件中使用正则表达式搜索
func (s *ZipGrepSearcher) GrepRegexpInFile(fileName string, pattern string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: true,
	}
	for _, opt := range opts {
		opt(config)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, utils.Errorf("compile regexp failed: %s", err)
	}

	fc, err := s.getFileContent(fileName)
	if err != nil {
		return nil, err
	}

	return s.grepFileContent(fc, func(line string) bool {
		return re.MatchString(line)
	}, config, "regexp:"+pattern), nil
}

// GrepSubStringInFile 在指定文件中使用子字符串搜索
func (s *ZipGrepSearcher) GrepSubStringInFile(fileName string, substring string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: false,
	}
	for _, opt := range opts {
		opt(config)
	}

	fc, err := s.getFileContent(fileName)
	if err != nil {
		return nil, err
	}

	matcher := func(line string) bool {
		if config.CaseSensitive {
			return strings.Contains(line, substring)
		}
		return strings.Contains(strings.ToLower(line), strings.ToLower(substring))
	}

	return s.grepFileContent(fc, matcher, config, "substring:"+substring), nil
}

// GetCachedFiles 返回已缓存的文件名列表
func (s *ZipGrepSearcher) GetCachedFiles() []string {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	files := make([]string, 0, len(s.fileCache))
	for name := range s.fileCache {
		files = append(files, name)
	}
	return files
}

// GetCacheSize 返回缓存的大小（字节数）
func (s *ZipGrepSearcher) GetCacheSize() int {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	size := 0
	for _, fc := range s.fileCache {
		size += len(fc.content)
	}
	return size
}

// GetFileCount 返回 ZIP 中的文件总数
func (s *ZipGrepSearcher) GetFileCount() int {
	count := 0
	for _, file := range s.reader.File {
		if !file.FileInfo().IsDir() {
			count++
		}
	}
	return count
}

// ClearCache 清空缓存
func (s *ZipGrepSearcher) ClearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.fileCache = make(map[string]*fileContent)
	log.Infof("cache cleared for %s", s.zipFile)
}

// GetFileContent 获取文件的完整内容（用于外部访问）
func (s *ZipGrepSearcher) GetFileContent(fileName string) (string, error) {
	fc, err := s.getFileContent(fileName)
	if err != nil {
		return "", err
	}
	return fc.content, nil
}

// String 返回搜索器的描述信息
func (s *ZipGrepSearcher) String() string {
	return fmt.Sprintf("ZipGrepSearcher{file=%s, cached=%d/%d files, cacheSize=%d bytes}",
		s.zipFile, len(s.fileCache), s.GetFileCount(), s.GetCacheSize())
}
