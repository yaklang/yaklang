package vectorstore

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/test"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type MockEmbeddingClient struct {
	vocabulary  []string       // 存储所有唯一的、排序后的关键词
	wordToIndex map[string]int // 从词到其在向量中索引的映射
	dimension   int            // 向量的维度，即词典大小
}

func (c *MockEmbeddingClient) Embedding(text string) ([]float32, error) { // 由于单次边界的问题，尽量采用中文
	vector := make([]float32, c.dimension)
	textLower := strings.ToLower(text)
	for word, index := range c.wordToIndex {
		re := regexp.MustCompile(regexp.QuoteMeta(word))
		matches := re.FindAllStringIndex(textLower, -1)
		vector[index] = float32(len(matches)) // 将词频作为向量的值
	}
	return vector, nil
}

func (c *MockEmbeddingClient) EmbeddingRaw(text string) ([][]float32, error) {
	vec, err := c.Embedding(text)
	if err != nil {
		return nil, err
	}
	return [][]float32{vec}, nil
}

func NewMockEmbedding(vocabulary []string) (*MockEmbeddingClient, error) {
	if len(vocabulary) == 0 {
		return nil, utils.Errorf("词典不能为空")
	}
	uniqueWords := make(map[string]bool)
	log.Infof("词典大小原始: %d", len(vocabulary))
	for _, word := range vocabulary {
		uniqueWords[word] = true
	}
	sortedVocab := make([]string, 0, len(uniqueWords))
	for word := range uniqueWords {
		sortedVocab = append(sortedVocab, word)
	}
	sort.Strings(sortedVocab)
	// 2. 创建 word -> index 映射
	wordToIndex := make(map[string]int, len(sortedVocab))
	for i, word := range sortedVocab {
		wordToIndex[word] = i
	}
	// 3. 初始化实例
	m := &MockEmbeddingClient{
		vocabulary:  sortedVocab,
		wordToIndex: wordToIndex,
		dimension:   len(sortedVocab),
	}
	log.Infof("MockEmbedding 初始化完成。")
	log.Infof("词典去重大小 (向量维度): %d", m.dimension)
	return m, nil
}

var Vocabulary1024 = []string{
	// 科技 & 计算机 (Tech & Computing)
	"algorithm", "api", "application", "authentication", "backend", "binary",
	"blockchain", "browser", "bug", "cache", "cloud", "code", "compiler",
	"container", "cookie", "cpu", "cybersecurity", "dashboard", "database",
	"debug", "deployment", "desktop", "developer", "devops", "digital",
	"domain", "download", "encryption", "engine", "ethernet", "exception",
	"firewall", "firmware", "framework", "frontend", "function", "gateway",
	"gpu", "hacker", "hardware", "hosting", "html", "http", "https",
	"hyperlink", "icon", "ide", "index", "infrastructure", "input",
	"integer", "interface", "internet", "iot", "ip", "java", "javascript",
	"json", "kernel", "keyboard", "keyword", "laptop", "library", "linux",
	"login", "logic", "loop", "machine", "malware", "memory", "metadata",
	"microservice", "middleware", "mobile", "model", "module", "monitor",
	"network", "node", "notification", "object", "offline", "online",
	"opensource", "operator", "optimization", "output", "package",
	"packet", "page", "parameter", "password", "patch", "payload",
	"performance", "peripheral", "phishing", "pixel", "platform", "plugin",
	"pointer", "policy", "port", "portal", "post", "privacy", "process",
	"processor", "protocol", "proxy", "python", "query", "queue", "ram",
	"raspberry", "react", "recursive", "redundancy", "registry", "repository",
	"request", "response", "rest", "router", "runtime", "saas", "sandbox",
	"scalability", "scanner", "schema", "script", "sdk", "security",
	"sensor", "server", "session", "software", "source", "spam", "sql",
	"ssd", "ssl", "stack", "storage", "stream", "string", "subnet",
	"superuser", "svg", "switch", "synchronize", "syntax", "system",
	"table", "tag", "tcp", "template", "terminal", "thread", "token",
	"toolkit", "traffic", "transaction", "trojan", "udp", "ui", "unix",
	"upload", "url", "user", "ux", "variable", "version", "virtual",
	"virus", "vpn", "vulnerability", "web", "websocket", "widget", "wifi",
	"windows", "wireless", "xml", "yaml", "zip",
	// 中文高频词 (Chinese High-Frequency Words)
	"人工智能", "数据", "分析", "用户", "模型", "系统", "网络",
	"云", "计算", "应用", "服务", "产品", "设计", "开发", "测试", "项目",
	"管理", "团队", "协作", "流程", "架构", "性能", "优化", "部署", "监控",
	"日志", "报警", "配置", "版本", "控制", "代码", "仓库", "分支", "合并",
	"请求", "响应", "接口", "协议", "加密", "解密", "认证", "授权", "令牌",
	"会话", "缓存", "数据库", "查询", "索引", "事务", "存储", "备份", "恢复",
	"容器", "虚拟化", "集群", "负载", "均衡", "弹性", "扩展", "微服务", "消息",
	"队列", "订阅", "发布", "事件", "驱动", "函数", "脚本", "前端", "后端",
	"界面", "交互", "体验", "兼容性", "可访问性", "自动化", "运维", "持续集成",
	"交付", "商业", "智能", "报告", "图表", "可视化", "市场", "营销", "销售",
	"客户", "关系", "策略", "风险", "合规", "审计", "财务", "预算", "投资",
	"回报", "供应链", "物流", "库存", "生产", "质量", "标准", "创新", "专利",
	"研发", "教育", "培训", "课程", "学习", "健康", "医疗", "诊断", "治疗",
	"基因", "环保", "能源", "可持续", "发展", "城市", "交通", "规划", "建筑",
	"农业", "食品", "安全", "旅游", "文化", "艺术", "媒体", "新闻", "娱乐",
	"游戏", "社交", "社区", "内容", "消费", "零售", "电商", "支付", "金融",
	// 自然与科学 (Nature & Science)
	"acid", "adaptation", "alkaline", "alloy", "altitude", "aluminum",
	"amino", "ammonia", "amphibian", "analysis", "anatomy", "animal",
	"antenna", "antimony", "apex", "aquatic", "argon", "artery", "asteroid",
	"atmosphere", "atom", "axis", "bacteria", "balance", "bark", "base",
	"beam", "behavior", "biology", "biome", "biosphere", "bird", "boiling",
	"botany", "calcium", "canopy", "capillary", "carbohydrate",
	"carbon", "catalyst", "cell", "charge", "chemical", "chemistry",
	"chlorophyll", "chromosome", "circuit", "climate", "comet",
	"condensation", "conduction", "conservation", "constellation",
	"continent", "core", "cosmos", "crater", "crystal", "current", "cycle",
	"cytoplasm", "decay", "delta", "density", "desert", "diffusion",
	"digestion", "dna", "dormant", "drought", "dwarf", "dynamics", "earth",
	"eclipse", "ecology", "ecosystem", "egg", "electric", "electron",
	"element", "embryo", "energy", "entropy", "environment", "enzyme",
	"epicenter", "equation", "equator", "erosion", "estuary", "eukaryote",
	"evaporation", "evolution", "excretion", "exosphere", "experiment",
	"extinction", "fault", "fauna", "fermentation", "fertilizer", "fission",
	"flora", "flower", "focus", "fog", "food", "force", "fossil", "frequency",
	"friction", "fungus", "fusion", "galaxy", "gas", "gene", "genetics",
	"genus", "geology", "geothermal", "germ", "glacier", "glucose",
	"gravity", "greenhouse", "growth", "habitat", "heat", "helium",
	"hemisphere", "herbivore", "heredity", "hormone", "humidity", "humus",
	"hybrid", "hydrocarbon", "hydrogen", "hypothesis", "ice", "igneous",
	"inertia", "infrared", "inheritance", "inorganic", "insect", "instinct",
	"insulator", "ion", "iron", "isotope", "joule", "jupiter", "kinetic",
	"kingdom", "krypton", "laboratory", "lake", "larva", "laser", "lava",
	"leaf", "lens", "lichen", "life", "light", "lightning", "liquid",
	"lithium", "lithosphere", "lunar", "lung", "magma", "magnet", "mammal",
	"mantle", "marine", "mars", "mass", "matter", "meiosis", "membrane",
	"mercury", "metabolism", "metal", "metamorphosis", "meteor",
	"methane", "microbe", "microscope", "migration", "milky", "mineral",
	"mitochondria", "mitosis", "mixture", "molecule", "momentum", "moon",
	"moss", "motion", "mountain", "mutation", "natural", "nebula", "neon",
	"nerve", "neuron", "neutral", "neutron", "niche", "nickel", "nitrogen",
	"noble", "nuclear", "nucleus", "nutrient", "nymph", "ocean", "ohm",
	"omnivore", "orbit", "organ", "organism", "osmosis", "ovum", "oxidation",
	"oxygen", "ozone", "paleontology", "parasite", "particle", "penumbra",
	"perennial", "permeable", "ph", "phenotype", "photosynthesis", "phylum",
	"physics", "pistil", "planet", "plankton", "plant", "plasma", "plate",
	"pollen", "pollination", "polymer", "population", "potential", "power",
	"predator", "pressure", "prey", "primate", "prism", "prokaryote",
	"protein", "proton", "pupa", "quark", "radiation", "radical", "radioactive",
	"rain", "reaction", "recessive", "recycle", "reflection", "refraction",
	"reproduction", "reptile", "resistance", "respiration", "ribosome",
	"richter", "river", "rna", "rock", "root", "rotation", "rust",
	"salinity", "salt", "satellite", "saturated", "savanna", "science",
	"season", "sediment", "seed", "seismic", "shadow", "shell", "silicon",
	"smog", "soil", "solar", "solid", "solstice", "solubility", "solution",
	"solvent", "sound", "species", "spectrum", "speed", "sperm", "spore",
	"stamen", "star", "steam", "stem", "stigma", "stomata", "stratosphere",
	"substance", "sugar", "sun", "supernova", "symbiosis", "synthesis",
	"tectonics", "telescope", "temperature", "theory", "thermal",
	"thermometer", "tide", "tissue", "titanium", "toxic", "trace",
	"transpiration", "tropical", "troposphere", "tsunami", "tundra",
	"ultraviolet", "umbra", "universe", "uranium", "vacuum", "valley",
	"vapor", "vascular", "vegetation", "vein", "velocity",
	"venus", "vertebrate", "vibration", "viscosity", "vitamin", "volcano",
	"volt", "volume", "water", "watt", "wave", "wavelength", "weather",
	"weight", "wetland", "wind", "xenon", "xylem", "year", "yeast", "zinc",
	"zodiac", "zone", "zoology", "zygote",
	// 商业与经济 (Business & Economy)
	"account", "acquisition", "advertising", "agenda", "agreement",
	"amortization", "angel", "annuity", "arbitrage", "asset",
	"audit", "automation", "b2b", "b2c", "bankruptcy", "bargain",
	"barter", "bear", "benchmark", "beneficiary", "beverage", "bid", "bill",
	"board", "bond", "bonus", "bookkeeping", "brand", "break-even", "broker",
	"budget", "bull", "business", "buyout", "capital", "capitalism",
	"cash", "catalog", "ceo", "cfo", "chain", "charity", "charter", "check",
	"claim", "client", "collaboration", "collateral", "commission",
	"commodity", "company", "compensation", "competition", "competitor",
	"compliance", "compound", "conglomerate", "consensus", "consultant",
	"consumer", "contract", "copyright", "corporation", "cost", "coupon",
	"credit", "crowdfunding", "currency", "customer", "deadline", "debenture",
	"debt", "decision", "default", "deficit", "deflation", "delivery",
	"demand", "demographics", "department", "deposit", "depreciation",
	"depression", "deregulation", "derivative", "design", "development",
	"diligence", "director", "disability", "discount", "distribution",
	"diversification", "dividend", "downsize", "due", "e-commerce",
	"earn", "earnings", "economics", "efficiency", "employee", "engagement",
	"enterprise", "entrepreneur", "equity", "escrow", "estate", "estimate",
	"ethics", "euro", "evaluate", "exchange", "executive", "expense",
	"export", "facility", "factory", "federal", "feedback", "finance",
	"firm", "fiscal", "fixed", "forecast", "foreign", "franchise", "fraud",
	"fund", "futures", "gain", "gdp", "globalization", "goal", "goods",
	"goodwill", "government", "grant", "gross", "guarantee",
	"headquarters", "hedge", "hire", "holding", "hr", "human", "import",
	"incentive", "income", "incorporation", "indemnity",
	"individual", "industry", "inflation", "information",
	"initial", "innovation", "insolvency", "inspection", "insurance",
	"intellectual", "interest", "international", "inventory", "invest",
	"invoice", "ipo", "irrevocable", "issue", "item", "jargon", "job",
	"joint", "journal", "judgment", "junior", "junk", "jurisdiction",
	"just-in-time", "keiretsu", "kickback", "know-how", "labor", "law",
	"lawsuit", "layoff", "lease", "ledger", "legacy", "legal", "leverage",
	"liability", "license", "lien", "limited", "line",
	"litigation", "loan", "local", "logo", "logistics", "long-term", "loss",
	"loyal", "luxury", "macroeconomics", "management", "manufacture",
	"margin", "market", "marketing", "master", "maturity", "mb",
	"media", "meeting", "memo", "mentor", "merchandise", "merger",
	// 情感 & 心理 (Emotion & Psychology)
	"喜悦", "愤怒", "悲伤", "恐惧", "惊讶", "焦虑", "乐观", "悲观", "自信", "谦逊",
	"同情", "动机", "灵感", "压力", "放松", "冥想", "认知", "记忆", "潜意识", "直觉",
	// 文化 & 艺术 (Culture & Art)
	"传统", "哲学", "历史", "文学", "诗歌", "戏剧", "电影", "音乐", "绘画", "书法",
	"雕塑", "摄影", "遗产", "博物馆", "展览", "节日", "仪式",
	// 社会 & 生活 (Society & Life)
	"家庭", "公民", "法律", "政策", "公平", "正义", "自由", "责任", "权利",
	"时尚", "美食", "运动", "旅行", "公益", "志愿者",
	// 抽象概念 (Abstract Concepts)
	"时间", "空间", "现实", "梦想", "原因", "结果", "概念", "逻辑", "结构", "模式",
	"变化", "稳定", "复杂", "简单", "本质", "现象", "矛盾", "统一",
	// 具体名词 & 科技补充 (Specific Nouns & Tech Additions)
	"手机", "电脑", "智能手表", "无人机", "机器人", "物联网", "大数据", "区块链",
	"操作系统", "应用程序", "算法", "服务器", "客户端", "域名", "防火墙",
	"电子商务", "数字货币", "股票", "基金", "保险", "房地产", "制造业",
	"新能源", "自动驾驶", "生物科技", "量子计算",
	"冲积扇", "喀斯特", "地平线", "玄武岩", "冰碛", "三角洲",
}

// NewDefaultMockEmbedding 创建一个默认的 MockEmbeddingClient 实例，使用预定义的词典，向量纬度为1024
func NewDefaultMockEmbedding() *MockEmbeddingClient {
	client, _ := NewMockEmbedding(Vocabulary1024)
	return client
}

// GenerateRandomText 从词典中随机选择词汇来生成一段文本。
func (c *MockEmbeddingClient) GenerateRandomText(wordCount int) string {
	return strings.Join(c.GenerateRandomWord(wordCount), " ")
}

func (c *MockEmbeddingClient) GenerateRandomWord(wordCount int) []string {
	if wordCount <= 0 {
		return nil
	}
	source := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(source)
	if wordCount > c.dimension {
		wordCount = c.dimension
	}
	shuffledVocab := make([]string, c.dimension)
	copy(shuffledVocab, c.vocabulary)
	rng.Shuffle(len(shuffledVocab), func(i, j int) {
		shuffledVocab[i], shuffledVocab[j] = shuffledVocab[j], shuffledVocab[i]
	})
	selectedWords := shuffledVocab[:wordCount]
	return selectedWords
}

// GenerateSimilarText 生成一个与基础文本相似度高于或等于阈值的文本。
func (c *MockEmbeddingClient) GenerateSimilarText(baseText string, threshold float64) (string, error) {
	if threshold < 0.0 || threshold > 1.0 {
		return "", fmt.Errorf("阈值必须在 [0.0, 1.0] 之间")
	}
	baseVec, _ := c.Embedding(baseText)
	baseNorm := hnsw.Norm(baseVec)
	if baseNorm == 0 {
		return "", fmt.Errorf("基础文本不包含任何词典中的关键词，无法生成相似文本")
	}

	var baseWords []string
	for _, word := range c.vocabulary {
		if strings.Contains(baseText, word) {
			baseWords = append(baseWords, word)
		}
	}

	newWords := make(map[string]bool)
	for _, w := range baseWords {
		newWords[w] = true
	}
	var generatedText string
	var currentSimilarity float64
	// 随机化添加顺序
	shuffledVocab := make([]string, c.dimension)
	copy(shuffledVocab, c.vocabulary)

	source := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(source)
	rng.Shuffle(len(shuffledVocab), func(i, j int) {
		shuffledVocab[i], shuffledVocab[j] = shuffledVocab[j], shuffledVocab[i]
	})
	maxIterations := c.dimension * 3
	for i := 0; i < maxIterations; i++ {
		newVec, _ := c.Embedding(generatedText)
		sim, _ := hnsw.CosineSimilarity(baseVec, newVec)
		if sim >= threshold {
			return generatedText, nil
		}
		currentSimilarity = sim

		// 策略：优先重复 baseText 中的词来提高相似度，如果不够再添加新词
		if rng.Float64() < 0.7 && len(baseWords) > 0 { // 70% 的概率重复旧词
			wordToRepeat := baseWords[rng.Intn(len(baseWords))]
			generatedText += " " + wordToRepeat
		} else { // 30%的概率添加一个不相关的词（这会轻微降低相似度，但增加文本多样性）
			var otherWords []string
			baseWordSet := make(map[string]bool)
			for _, w := range baseWords {
				baseWordSet[w] = true
			}
			for _, v := range c.vocabulary {
				if !baseWordSet[v] {
					otherWords = append(otherWords, v)
				}
			}
			if len(otherWords) > 0 {
				generatedText += " " + otherWords[rng.Intn(len(otherWords))]
			}
		}
	}
	return "", utils.Errorf("无法生成相似度 > %.2f 的文本，当前最大可达 %.4f", threshold, currentSimilarity)
}

type MockEmbedder struct {
	MockEmbedderFunc func(text string) ([]float32, error)
}

func NewMockEmbedder(f func(text string) ([]float32, error)) EmbeddingClient {
	return &MockEmbedder{
		MockEmbedderFunc: f,
	}
}

// Embedding 模拟实现 EmbeddingClient 接口
func (m *MockEmbedder) Embedding(text string) ([]float32, error) {
	return m.MockEmbedderFunc(text)
}

// EmbeddingRaw 返回单个向量的二维数组形式
func (m *MockEmbedder) EmbeddingRaw(text string) ([][]float32, error) {
	vec, err := m.MockEmbedderFunc(text)
	if err != nil {
		return nil, err
	}
	return [][]float32{vec}, nil
}

// getMockRagDataForTest 在当前函数中缓存embedding数据，避免每次都读取文件
func getMockRagDataForTest() (func(text string) ([]float32, error), error) {
	content, err := test.FS.ReadFile("mock_embedding_data.json")
	if err != nil {
		return nil, utils.Errorf("failed to read embedding data: %v", err)
	}
	var embeddingData map[string][]float32
	err = json.Unmarshal(content, &embeddingData)
	if err != nil {
		return nil, utils.Errorf("failed to unmarshal embedding data: %v", err)
	}
	// 返回一个函数，用于获取嵌入数据
	return func(text string) ([]float32, error) {
		embedding, ok := embeddingData[text]
		if !ok {
			return nil, utils.Errorf("text not found: %s", text)
		}
		return embedding, nil
	}, nil
}
