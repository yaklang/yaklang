package ziputil

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"runtime"
	"sort"
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

	// 路径过滤
	IncludePathSubString []string // 包含路径子串（任一匹配即可）
	ExcludePathSubString []string // 排除路径子串（任一匹配即排除）
	IncludePathRegexp    []string // 包含路径正则（任一匹配即可）
	ExcludePathRegexp    []string // 排除路径正则（任一匹配即排除）
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

func WithGrepCaseSensitive(i ...bool) GrepOption {
	return func(c *GrepConfig) {
		if len(i) > 0 {
			c.CaseSensitive = i[0]
			return
		}
		c.CaseSensitive = true
	}
}

// 路径过滤选项

func WithIncludePathSubString(patterns ...string) GrepOption {
	return func(c *GrepConfig) {
		c.IncludePathSubString = append(c.IncludePathSubString, patterns...)
	}
}

func WithExcludePathSubString(patterns ...string) GrepOption {
	return func(c *GrepConfig) {
		c.ExcludePathSubString = append(c.ExcludePathSubString, patterns...)
	}
}

func WithIncludePathRegexp(patterns ...string) GrepOption {
	return func(c *GrepConfig) {
		c.IncludePathRegexp = append(c.IncludePathRegexp, patterns...)
	}
}

func WithExcludePathRegexp(patterns ...string) GrepOption {
	return func(c *GrepConfig) {
		c.ExcludePathRegexp = append(c.ExcludePathRegexp, patterns...)
	}
}

type GrepResult struct {
	FileName      string   // 文件名
	LineNumber    int      // 行号
	Line          string   // 匹配的行
	ContextBefore []string // 前置上下文
	ContextAfter  []string // 后置上下文

	// RRF 相关字段
	Score       float64 // 搜索得分
	ScoreMethod string  // 搜索方法

	// 合并相关字段
	MatchedLines []int // 合并后的所有匹配行号
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
	}, config, "regexp:"+pattern)
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

	return grepZipContent(raw, matcher, config, "substring:"+substring)
}

// shouldIncludePath 判断路径是否应该被包含
func shouldIncludePath(path string, config *GrepConfig) bool {
	// 检查排除规则
	for _, pattern := range config.ExcludePathSubString {
		if strings.Contains(path, pattern) {
			return false
		}
	}

	for _, pattern := range config.ExcludePathRegexp {
		if matched, _ := regexp.MatchString(pattern, path); matched {
			return false
		}
	}

	// 检查包含规则（如果设置了包含规则，则必须匹配其中之一）
	if len(config.IncludePathSubString) > 0 {
		matched := false
		for _, pattern := range config.IncludePathSubString {
			if strings.Contains(path, pattern) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(config.IncludePathRegexp) > 0 {
		matched := false
		for _, pattern := range config.IncludePathRegexp {
			if m, _ := regexp.MatchString(pattern, path); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

func grepZipContent(raw interface{}, matcher func(string) bool, config *GrepConfig, method ...string) ([]*GrepResult, error) {
	scoreMethod := "default"
	if len(method) > 0 {
		scoreMethod = method[0]
	}
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

		// 应用路径过滤
		if !shouldIncludePath(file.Name, config) {
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

			fileResults := grepFile(f.Name, rc, matcher, config, scoreMethod)
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

func grepFile(filename string, r io.Reader, matcher func(string) bool, config *GrepConfig, scoreMethod string) []*GrepResult {
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
				FileName:    filename,
				LineNumber:  lineNumber,
				Line:        line,
				ScoreMethod: scoreMethod,
				Score:       1.0 / float64(lineNumber+1), // 默认得分
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

// String 返回 GrepResult 的人类可读格式
func (g *GrepResult) String() string {
	var sb strings.Builder

	// 文件名和行号
	sb.WriteString(fmt.Sprintf("%s:%d", g.FileName, g.LineNumber))

	// 如果有合并的行号，显示所有匹配行
	if len(g.MatchedLines) > 0 {
		sb.WriteString(" [")
		for i, line := range g.MatchedLines {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("%d", line))
		}
		sb.WriteString("]")
	}

	sb.WriteString("\n")

	// 显示前置上下文
	if len(g.ContextBefore) > 0 {
		for i, line := range g.ContextBefore {
			lineNum := g.LineNumber - len(g.ContextBefore) + i
			sb.WriteString(fmt.Sprintf("%6d  | %s\n", lineNum, line))
		}
	}

	// 显示匹配行（用 > 标记）
	marker := ">"
	if len(g.MatchedLines) > 0 {
		marker = "*" // 如果是合并的结果，用 * 标记
	}
	sb.WriteString(fmt.Sprintf("%6d %s | %s\n", g.LineNumber, marker, g.Line))

	// 显示额外的匹配行
	if len(g.MatchedLines) > 0 {
		// 需要在上下文中标记其他匹配行
		// 这里简化处理，只在后置上下文中查找
	}

	// 显示后置上下文
	if len(g.ContextAfter) > 0 {
		for i, line := range g.ContextAfter {
			lineNum := g.LineNumber + i + 1
			// 检查这行是否也是匹配行
			isMatched := false
			for _, matchedLine := range g.MatchedLines {
				if matchedLine == lineNum {
					isMatched = true
					break
				}
			}

			if isMatched {
				sb.WriteString(fmt.Sprintf("%6d * | %s\n", lineNum, line))
			} else {
				sb.WriteString(fmt.Sprintf("%6d  | %s\n", lineNum, line))
			}
		}
	}

	// 如果有得分信息，显示出来
	if g.Score > 0 {
		sb.WriteString(fmt.Sprintf("Score: %.4f", g.Score))
		if g.ScoreMethod != "" {
			sb.WriteString(fmt.Sprintf(" (Method: %s)", g.ScoreMethod))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetUUID 实现 RRFScoredData 接口，返回唯一标识
func (g *GrepResult) GetUUID() string {
	return fmt.Sprintf("%s:%d", g.FileName, g.LineNumber)
}

// GetScore 实现 RRFScoredData 接口，返回得分
func (g *GrepResult) GetScore() float64 {
	if g.Score > 0 {
		return g.Score
	}
	// 默认得分：根据行号倒序（越靠前得分越高）
	return 1.0 / float64(g.LineNumber+1)
}

// GetScoreMethod 实现 RRFScoredData 接口，返回评分方法
func (g *GrepResult) GetScoreMethod() string {
	if g.ScoreMethod != "" {
		return g.ScoreMethod
	}
	return "default"
}

// CanMerge 判断两个 GrepResult 是否可以合并
// 合并条件：同一个文件，且一个匹配行在另一个的上下文范围内
func (g *GrepResult) CanMerge(other *GrepResult) bool {
	if g.FileName != other.FileName {
		return false
	}

	// 计算上下文范围
	g1Start := g.LineNumber - len(g.ContextBefore)
	g1End := g.LineNumber + len(g.ContextAfter)
	g2Start := other.LineNumber - len(other.ContextBefore)
	g2End := other.LineNumber + len(other.ContextAfter)

	// 检查是否有重叠
	return (g.LineNumber >= g2Start && g.LineNumber <= g2End) ||
		(other.LineNumber >= g1Start && other.LineNumber <= g1End) ||
		(g1Start >= g2Start && g1Start <= g2End) ||
		(g2Start >= g1Start && g2Start <= g1End)
}

// Merge 合并两个 GrepResult
func (g *GrepResult) Merge(other *GrepResult) *GrepResult {
	if !g.CanMerge(other) {
		return g
	}

	// 创建新的结果
	merged := &GrepResult{
		FileName:     g.FileName,
		ScoreMethod:  g.ScoreMethod,
		MatchedLines: []int{},
	}

	// 使用较小的行号作为主行号
	if g.LineNumber < other.LineNumber {
		merged.LineNumber = g.LineNumber
		merged.Line = g.Line
	} else {
		merged.LineNumber = other.LineNumber
		merged.Line = other.Line
	}

	// 记录所有匹配的行号
	matchedSet := make(map[int]bool)
	matchedSet[g.LineNumber] = true
	matchedSet[other.LineNumber] = true

	// 添加已有的匹配行
	for _, line := range g.MatchedLines {
		matchedSet[line] = true
	}
	for _, line := range other.MatchedLines {
		matchedSet[line] = true
	}

	for line := range matchedSet {
		merged.MatchedLines = append(merged.MatchedLines, line)
	}

	// 排序匹配行
	sort.Ints(merged.MatchedLines)

	// 合并上下文
	minLine := merged.LineNumber
	maxLine := merged.LineNumber

	for _, line := range merged.MatchedLines {
		if line < minLine {
			minLine = line
		}
		if line > maxLine {
			maxLine = line
		}
	}

	// 构建合并后的上下文
	// 简化处理：使用范围更大的那个的上下文
	if len(g.ContextBefore) >= len(other.ContextBefore) {
		merged.ContextBefore = g.ContextBefore
	} else {
		merged.ContextBefore = other.ContextBefore
	}

	if len(g.ContextAfter) >= len(other.ContextAfter) {
		merged.ContextAfter = g.ContextAfter
	} else {
		merged.ContextAfter = other.ContextAfter
	}

	// 合并得分（取平均值）
	merged.Score = (g.Score + other.Score) / 2

	return merged
}

// MergeGrepResults 合并多个 GrepResult，将可以合并的结果合并在一起
func MergeGrepResults(results []*GrepResult) []*GrepResult {
	if len(results) <= 1 {
		return results
	}

	// 按文件名和行号排序
	sort.Slice(results, func(i, j int) bool {
		if results[i].FileName != results[j].FileName {
			return results[i].FileName < results[j].FileName
		}
		return results[i].LineNumber < results[j].LineNumber
	})

	merged := []*GrepResult{}
	current := results[0]

	for i := 1; i < len(results); i++ {
		if current.CanMerge(results[i]) {
			// 可以合并
			current = current.Merge(results[i])
		} else {
			// 不能合并，保存当前结果，开始新的
			merged = append(merged, current)
			current = results[i]
		}
	}

	// 添加最后一个
	merged = append(merged, current)

	return merged
}

// GrepPath 系列函数 - 搜索文件路径/文件名

// GrepPathRegexp 使用正则表达式搜索文件路径
func GrepPathRegexp(zipFile string, pattern string, opts ...GrepOption) ([]*GrepResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return GrepPathRawRegexp(raw, pattern, opts...)
}

// GrepPathSubString 使用子字符串搜索文件路径
func GrepPathSubString(zipFile string, substring string, opts ...GrepOption) ([]*GrepResult, error) {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return nil, utils.Errorf("read zip file failed: %s", err)
	}
	return GrepPathRawSubString(raw, substring, opts...)
}

// GrepPathRawRegexp 使用正则表达式在原始数据中搜索文件路径
func GrepPathRawRegexp(raw interface{}, pattern string, opts ...GrepOption) ([]*GrepResult, error) {
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

	return grepPathContent(raw, func(path string) bool {
		return re.MatchString(path)
	}, config, "path_regexp:"+pattern)
}

// GrepPathRawSubString 使用子字符串在原始数据中搜索文件路径
func GrepPathRawSubString(raw interface{}, substring string, opts ...GrepOption) ([]*GrepResult, error) {
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

	return grepPathContent(raw, matcher, config, "path_substring:"+substring)
}

// grepPathContent 在文件路径中搜索
func grepPathContent(raw interface{}, matcher func(string) bool, config *GrepConfig, method string) ([]*GrepResult, error) {
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

	var results []*GrepResult
	fileIndex := 0

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		fileIndex++

		// 应用路径过滤（对于 GrepPath，过滤是可选的）
		if !shouldIncludePath(file.Name, config) {
			continue
		}

		// 检查路径是否匹配
		if matcher(file.Name) {
			result := &GrepResult{
				FileName:    file.Name,
				LineNumber:  0, // 路径搜索没有行号概念
				Line:        file.Name,
				ScoreMethod: method,
				Score:       1.0 / float64(fileIndex+1), // 基于文件顺序的得分
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
