package aimem

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
)

// KeywordNormalizer 关键词规范化器 - 支持中英文混合处理
type KeywordNormalizer struct {
	// 停用词映射 - 中英文通用
	stopWords map[string]bool
	// 同义词映射
	synonymMap map[string]string
	// 中文分词缓存（可选）
	segmenter *chineseSegmenter
}

// chineseSegmenter 简单的中文分词器
type chineseSegmenter struct {
	// 常用词库
	dictionary map[string]bool
}

// NewKeywordNormalizer 创建新的关键词规范化器
func NewKeywordNormalizer() *KeywordNormalizer {
	kn := &KeywordNormalizer{
		stopWords:  getStopWords(),
		synonymMap: getSynonymMap(),
		segmenter:  newChineseSegmenter(),
	}
	return kn
}

// getStopWords 返回中英文停用词集合
func getStopWords() map[string]bool {
	stopWords := map[string]bool{
		// 英文停用词
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "up": true, "about": true,
		"into": true, "through": true, "during": true, "as": true, "is": true,
		"was": true, "are": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "can": true, "this": true, "that": true,
		"these": true, "those": true, "i": true, "you": true, "he": true,
		"she": true, "it": true, "we": true, "they": true, "what": true,
		"which": true, "who": true, "when": true, "where": true, "why": true,
		"how": true, "all": true, "each": true, "every": true, "both": true,
		"any": true, "some": true, "no": true, "nor": true, "not": true,

		// 中文停用词
		"的": true, "一": true, "是": true, "在": true, "了": true, "和": true,
		"人": true, "这": true, "中": true, "大": true, "为": true, "上": true,
		"个": true, "国": true, "我": true, "以": true, "要": true, "他": true,
		"时": true, "来": true, "用": true, "们": true, "生": true, "到": true,
		"作": true, "地": true, "于": true, "出": true, "就": true, "分": true,
		"对": true, "成": true, "会": true, "可": true, "主": true, "发": true,
		"年": true, "动": true, "同": true, "工": true, "也": true, "能": true,
		"下": true, "现": true, "之": true, "最": true, "新": true, "过": true,
		"好": true, "家": true, "名": true, "手": true, "面": true, "见": true,
		"书": true, "多": true, "情": true, "号": true, "学": true, "高": true,
		"认": true, "世": true, "行": true, "期": true, "法": true, "着": true,
		"去": true, "种": true, "制": true, "实": true, "样": true, "资": true,
		"比": true, "向": true, "那": true, "给": true, "她": true, "后": true,
		"里": true, "做": true, "让": true, "通": true, "并": true,
		"始": true, "全": true, "系": true, "前": true, "其": true, "具": true,
		"条": true, "内": true, "十": true, "又": true, "有": true, "者": true,
	}
	return stopWords
}

// getSynonymMap 返回同义词映射
func getSynonymMap() map[string]string {
	return map[string]string{
		// 技术相关
		"开发": "编程", "代码": "编程", "写代码": "编程",
		"编码": "编程", "编写": "编程",
		"错误": "问题", "bug": "问题", "缺陷": "问题",
		"问题": "问题", "issue": "问题",
		"修复": "解决", "解决": "解决", "fix": "解决",
		"改进": "优化", "优化": "优化", "性能": "优化",
		"测试": "测试", "单元测试": "测试", "集成测试": "测试",
		"部署": "发布", "发布": "发布", "上线": "发布",
		"说明": "文档", "文档": "文档",
		"工具": "工具", "软件": "工具", "应用": "工具",

		// 安全相关同义词
		"渗透": "渗透测试", "渗透测试": "渗透测试", "安全测试": "渗透测试",
		"漏洞": "漏洞", "漏洞检测": "漏洞",
		"扫描": "扫描",

		// Yaklang特定功能同义词
		"脚本": "脚本", "yak脚本": "脚本", "脚本执行": "脚本",
		"指纹": "指纹识别", "指纹识别": "指纹识别", "指纹库": "指纹识别",
		"端口": "端口扫描", "端口扫描": "端口扫描", "端口探测": "端口扫描",
		"爆破": "爆破", "弱口令": "爆破", "弱密码": "爆破", "密码爆破": "爆破",
		"中间人": "中间人攻击", "劫持": "中间人攻击", "mitm": "中间人攻击",
		"代理": "代理", "http代理": "代理", "流量代理": "代理",
		"拦截": "拦截", "流量拦截": "拦截", "请求拦截": "拦截",
		"证书": "证书", "ssl证书": "证书", "https证书": "证书",
		"爬虫": "爬虫", "网站爬虫": "爬虫", "网页爬虫": "爬虫",
		"资产": "资产", "资产发现": "资产", "资产扫描": "资产",
		"子域": "子域名", "子域名": "子域名", "子域枚举": "子域名",
		"dns": "dns", "dns查询": "dns", "dns枚举": "dns",
		"存活": "存活检测", "存活检测": "存活检测", "主机存活": "存活检测",
		"代码分析": "代码分析", "静态分析": "代码分析", "ssa": "代码分析",
		"虚拟机": "虚拟机", "vm": "虚拟机", "yakvirtualmachine": "虚拟机",
		"编译": "编译", "编译器": "编译", "编译执行": "编译",
		"调试": "调试", "调试器": "调试", "单步调试": "调试",

		// 通用同义词
		"api": "接口", "接口": "接口",
		"db": "数据库", "数据库": "数据库", "database": "数据库",
		"web": "网络", "网络": "网络", "网站": "网络",
		"云": "云计算", "云计算": "云计算", "cloud": "云计算",
		"安全": "安全", "防护": "安全",
		"用户": "用户", "user": "用户", "people": "用户",
		"系统": "系统", "system": "系统",
		"效率": "性能", "速度": "性能",

		// 中英混合
		"python": "python编程", "golang": "golang编程", "java": "java编程",
		"javascript": "js编程", "typescript": "ts编程",
		"react": "前端框架", "vue": "前端框架", "angular": "前端框架",
		"spring": "java框架", "express": "nodejs框架",
		"yaklang": "yaklang编程", "yak": "yaklang编程",
	}
}

// newChineseSegmenter 创建中文分词器
func newChineseSegmenter() *chineseSegmenter {
	return &chineseSegmenter{
		dictionary: getCommonChineseWords(),
	}
}

// getCommonChineseWords 返回常用中文词汇
func getCommonChineseWords() map[string]bool {
	return map[string]bool{
		// 技术词汇
		"编程": true, "开发": true, "代码": true, "算法": true, "数据结构": true,
		"系统": true, "应用": true, "软件": true, "硬件": true, "网络": true,
		"数据库": true, "接口": true, "测试": true, "部署": true, "性能": true,
		"优化": true, "安全": true, "加密": true, "认证": true, "授权": true,
		"服务器": true, "客户端": true, "前端": true, "后端": true, "全栈": true,
		"框架": true, "库": true, "工具": true, "插件": true, "扩展": true,
		"云": true, "容器": true, "微服务": true, "架构": true, "模式": true,

		// 安全测试相关词汇
		"漏洞": true, "扫描": true, "渗透": true, "渗透测试": true,
		"安全测试": true, "漏洞检测": true, "安全审计": true, "威胁": true,
		"攻击": true, "防护": true, "加固": true, "加强": true,
		"脆弱": true, "弱点": true, "缺陷": true, "风险": true,
		"payload": true, "注入": true, "绕过": true, "绕过防护": true,

		// Yaklang/Yakit 特定功能词汇
		"脚本": true, "编写": true, "执行": true, "运行": true,
		"指纹": true, "指纹识别": true, "指纹库": true,
		"服务": true, "服务识别": true, "服务扫描": true,
		"端口": true, "端口扫描": true, "端口服务": true,
		"爆破": true, "弱口令": true, "密码": true, "爆破工具": true,
		"中间人": true, "劫持": true, "代理": true, "拦截": true,
		"流量": true, "流量分析": true, "流量拦截": true,
		"证书": true, "tls": true, "https": true, "ssl": true,
		"加密协议": true, "协议": true, "协议分析": true,

		// 漏洞和风险相关
		"sql注入": true, "xss": true, "csrf": true, "rce": true,
		"远程执行": true, "代码执行": true, "命令执行": true,
		"目录遍历": true, "文件包含": true, "文件上传": true,
		"认证绕过": true, "访问控制": true, "权限提升": true,
		"信息泄露": true, "数据泄露": true, "敏感信息": true,
		"xml": true, "json": true, "反序列化": true,
		"模板注入": true, "表达式注入": true, "ognl": true,

		// 扫描和侦查
		"资产": true, "资产管理": true, "资产发现": true,
		"子域": true, "子域名": true, "子域名枚举": true,
		"域名": true, "dns": true, "dns查询": true,
		"ip": true, "ip地址": true, "网段": true, "网段扫描": true,
		"存活": true, "存活检测": true, "ping": true,
		"爬虫": true, "网站爬虫": true, "链接爬取": true,
		"Web": true, "http": true, "http代理": true,

		// Yaklang 运行时和框架
		"虚拟机": true, "vm": true, "解释器": true, "字节码": true,
		"语言": true, "编程语言": true, "dsl": true, "领域语言": true,
		"编译": true, "编译器": true, "词法分析": true, "语法分析": true,
		"ssa": true, "代码分析": true, "静态分析": true,
		"调试": true, "调试器": true, "断点": true, "跟踪": true,

		// 常见命令和操作
		"命令": true, "命令行": true, "cli": true, "终端": true,
		"参数": true, "选项": true, "标志": true, "配置": true,
		"输出": true, "结果": true, "日志": true, "记录": true,

		// 通用词汇
		"用户": true, "功能": true, "特性": true, "需求": true, "问题": true,
		"解决": true, "改进": true, "更新": true, "版本": true, "文档": true,
		"教程": true, "示例": true, "实践": true, "经验": true, "知识": true,
		"案例": true, "场景": true, "使用": true, "集成": true,
	}
}

// NormalizeKeyword 规范化单个关键词
// 处理：转换大小写、移除特殊字符、处理同义词
func (kn *KeywordNormalizer) NormalizeKeyword(keyword string) string {
	if keyword == "" {
		return ""
	}

	// 1. 去除首尾空格
	keyword = strings.TrimSpace(keyword)

	// 2. 转换为小写（英文部分）
	keyword = strings.ToLower(keyword)

	// 3. 移除特殊字符但保留中文
	keyword = removeSpecialChars(keyword)

	// 4. 处理同义词映射
	if synonym, ok := kn.synonymMap[keyword]; ok {
		return synonym
	}

	return keyword
}

// ExtractKeywords 从文本中提取关键词（中英文混合）
func (kn *KeywordNormalizer) ExtractKeywords(text string) []string {
	if text == "" {
		return []string{}
	}

	var keywords []string
	keywordMap := make(map[string]bool) // 用于去重

	// 1. 分离中文和英文
	chineseTokens := kn.extractChineseTokens(text)
	englishTokens := extractEnglishTokens(text)

	// 2. 合并所有tokens
	allTokens := append(chineseTokens, englishTokens...)

	// 3. 规范化和过滤
	for _, token := range allTokens {
		if token == "" {
			continue
		}

		// 过滤停用词
		if kn.stopWords[strings.ToLower(token)] {
			continue
		}

		// 规范化
		normalized := kn.NormalizeKeyword(token)
		if normalized == "" || keywordMap[normalized] {
			continue
		}

		keywordMap[normalized] = true
		keywords = append(keywords, normalized)
	}

	return keywords
}

// extractChineseTokens 提取中文tokens
// 实现简单但有效的中文分词：基于字典匹配和字符长度启发式
func (kn *KeywordNormalizer) extractChineseTokens(text string) []string {
	var tokens []string
	runes := []rune(text)
	i := 0

	for i < len(runes) {
		if !isChinese(runes[i]) {
			i++
			continue
		}

		// 尝试从词库中找最长匹配
		found := false

		// 优先尝试长词（2-3个字）
		for length := 3; length >= 2; length-- {
			if i+length <= len(runes) {
				potential := string(runes[i : i+length])
				// 检查是否全是中文
				allChinese := true
				for _, r := range potential {
					if !isChinese(r) {
						allChinese = false
						break
					}
				}

				if allChinese && kn.segmenter.dictionary[potential] {
					tokens = append(tokens, potential)
					i += length
					found = true
					break
				}
			}
		}

		if !found {
			// 如果词库中没有匹配，尝试单个字符或按启发式拆分
			single := string(runes[i])
			// 过滤停用词的单个字符
			if !kn.stopWords[single] {
				tokens = append(tokens, single)
			}
			i++
		}
	}

	return tokens
}

// extractEnglishTokens 提取英文tokens
func extractEnglishTokens(text string) []string {
	var tokens []string

	// 使用正则表达式提取英文单词和数字
	re := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	matches := re.FindAllString(text, -1)

	for _, match := range matches {
		if len(match) > 2 { // 过滤长度小于3的单词
			tokens = append(tokens, match)
		}
	}

	return tokens
}

// isChinese 判断字符是否为中文
func isChinese(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// removeSpecialChars 移除特殊字符但保留中文和英文
func removeSpecialChars(s string) string {
	var result []rune
	for _, r := range s {
		if isChinese(r) || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			result = append(result, r)
		}
	}
	return string(result)
}

// MatchKeywords 进行关键词匹配，支持多种匹配策略
type KeywordMatcher struct {
	normalizer *KeywordNormalizer
}

// NewKeywordMatcher 创建关键词匹配器
func NewKeywordMatcher() *KeywordMatcher {
	return &KeywordMatcher{
		normalizer: NewKeywordNormalizer(),
	}
}

// MatchScore 计算两个文本的关键词匹配分数 (0.0-1.0)
func (km *KeywordMatcher) MatchScore(query string, content string) float64 {
	if query == "" || content == "" {
		return 0.0
	}

	queryKeywords := km.normalizer.ExtractKeywords(query)
	contentKeywords := km.normalizer.ExtractKeywords(content)

	if len(queryKeywords) == 0 || len(contentKeywords) == 0 {
		return 0.0
	}

	// 计算Jaccard相似度
	matches := 0
	for _, qk := range queryKeywords {
		for _, ck := range contentKeywords {
			if qk == ck {
				matches++
				break
			}
		}
	}

	union := len(queryKeywords) + len(contentKeywords) - matches
	if union == 0 {
		return 0.0
	}

	score := float64(matches) / float64(union)
	log.Debugf("keyword match score: query=%v, content=%v, matches=%d, union=%d, score=%.3f",
		queryKeywords, contentKeywords, matches, union, score)

	return score
}

// ContainsKeyword 检查内容是否包含查询中的关键词（至少一个）
// 支持部分匹配和中英混合
func (km *KeywordMatcher) ContainsKeyword(query string, content string) bool {
	queryKeywords := km.normalizer.ExtractKeywords(query)
	if len(queryKeywords) == 0 {
		return true // 如果查询中没有关键词，认为匹配
	}

	contentLower := strings.ToLower(content)
	for _, keyword := range queryKeywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// MatchAllKeywords 检查内容是否包含查询中的所有关键词
func (km *KeywordMatcher) MatchAllKeywords(query string, content string) bool {
	queryKeywords := km.normalizer.ExtractKeywords(query)
	if len(queryKeywords) == 0 {
		return true
	}

	contentLower := strings.ToLower(content)
	for _, keyword := range queryKeywords {
		if !strings.Contains(contentLower, strings.ToLower(keyword)) {
			return false
		}
	}

	return true
}

// ExpandKeywords 关键词扩展 - 使用同义词进行扩展
func (km *KeywordMatcher) ExpandKeywords(keywords []string) []string {
	expanded := make(map[string]bool)

	for _, kw := range keywords {
		expanded[kw] = true

		// 添加同义词
		if synonym, ok := km.normalizer.synonymMap[kw]; ok {
			expanded[synonym] = true
		}
	}

	var result []string
	for kw := range expanded {
		result = append(result, kw)
	}

	return result
}
