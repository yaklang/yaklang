package ziputil

import (
	"archive/zip"
	"bufio"
	"io"
	"io/ioutil"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

type GrepConfig struct {
	Limit         int  // 限制结果数量
	Context       int  // 上下文行数
	CaseSensitive bool // 是否区分大小写
}

type GrepOption func(*GrepConfig)

func WithGrepLimit(limit int) GrepOption {
	return func(c *GrepConfig) {
		c.Limit = limit
	}
}

func WithContext(context int) GrepOption {
	return func(c *GrepConfig) {
		c.Context = context
	}
}

func WithGrepCaseSensitive() GrepOption {
	return func(c *GrepConfig) {
		c.CaseSensitive = true
	}
}

type GrepResult struct {
	FileName      string   // 文件名
	LineNumber    int      // 行号
	Line          string   // 匹配的行
	ContextBefore []string // 前置上下文
	ContextAfter  []string // 后置上下文
}

// GrepRegexp 使用正则表达式在 ZIP 文件中搜索
func GrepRegexp(zipFile string, pattern string, opts ...GrepOption) ([]*GrepResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return GrepRawRegexp(raw, pattern, opts...)
}

// GrepSubString 使用子字符串在 ZIP 文件中搜索
func GrepSubString(zipFile string, substring string, opts ...GrepOption) ([]*GrepResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return GrepRawSubString(raw, substring, opts...)
}

// GrepRawRegexp 使用正则表达式在 ZIP 原始数据中搜索
func GrepRawRegexp(raw interface{}, pattern string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1, // 默认不限制
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

	return grepZipContent(raw, func(line string) bool {
		return re.MatchString(line)
	}, config)
}

// GrepRawSubString 使用子字符串在 ZIP 原始数据中搜索
func GrepRawSubString(raw interface{}, substring string, opts ...GrepOption) ([]*GrepResult, error) {
	config := &GrepConfig{
		Limit:         -1,
		Context:       0,
		CaseSensitive: false, // 默认不区分大小写
	}
	for _, opt := range opts {
		opt(config)
	}

	searchStr := substring
	matcher := func(line string) bool {
		if config.CaseSensitive {
			return strings.Contains(line, searchStr)
		}
		return strings.Contains(strings.ToLower(line), strings.ToLower(searchStr))
	}

	return grepZipContent(raw, matcher, config)
}

func grepZipContent(raw interface{}, matcher func(string) bool, config *GrepConfig) ([]*GrepResult, error) {
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

	var (
		results   []*GrepResult
		resultsMu sync.Mutex
		wg        sync.WaitGroup
		limitCh   chan struct{}
	)

	// 只有在设置了限制时才创建 limitCh
	if config.Limit > 0 {
		limitCh = make(chan struct{}, config.Limit)
	}

	// 计算并发数
	concurrency := runtime.NumCPU()
	if concurrency > 8 {
		concurrency = 8
	}
	semaphore := make(chan struct{}, concurrency)

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// 如果已达到限制，停止处理
		if config.Limit > 0 {
			select {
			case limitCh <- struct{}{}:
			default:
				log.Infof("grep limit reached, stopping search")
				goto done
			}
		}

		wg.Add(1)
		go func(f *zip.File) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			rc, err := f.Open()
			if err != nil {
				log.Errorf("open file %s in zip failed: %s", f.Name, err)
				return
			}
			defer rc.Close()

			fileResults := grepFile(f.Name, rc, matcher, config)
			if len(fileResults) > 0 {
				resultsMu.Lock()
				results = append(results, fileResults...)
				resultsMu.Unlock()
			}
		}(file)
	}

done:
	wg.Wait()
	if limitCh != nil {
		close(limitCh)
	}

	// 应用限制
	if config.Limit > 0 && len(results) > config.Limit {
		results = results[:config.Limit]
	}

	return results, nil
}

func grepFile(filename string, r io.Reader, matcher func(string) bool, config *GrepConfig) []*GrepResult {
	var results []*GrepResult
	scanner := bufio.NewScanner(r)
	lineNumber := 0
	var contextBuffer []string

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// 维护上下文缓冲区
		if config.Context > 0 {
			contextBuffer = append(contextBuffer, line)
			if len(contextBuffer) > config.Context*2+1 {
				contextBuffer = contextBuffer[1:]
			}
		}

		if matcher(line) {
			result := &GrepResult{
				FileName:   filename,
				LineNumber: lineNumber,
				Line:       line,
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

				// 后置上下文（先标记，后续行处理）
				// 简化处理：只在当前行后继续读取
				afterLines := []string{}
				for i := 0; i < config.Context && scanner.Scan(); i++ {
					lineNumber++
					afterLines = append(afterLines, scanner.Text())
					if config.Context > 0 {
						contextBuffer = append(contextBuffer, scanner.Text())
						if len(contextBuffer) > config.Context*2+1 {
							contextBuffer = contextBuffer[1:]
						}
					}
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
