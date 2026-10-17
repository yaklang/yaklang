package vectorstore

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type domainDoc struct {
	topic string
	text  string
}

type domainQuery struct {
	topic string
	text  string
	kind  string // exact, paraphrase, hard
}

type scoredDoc struct {
	id    string
	topic string
	text  string
	score float64
}

func TestRemoteEmbeddingRAGMetrics(t *testing.T) {
	if os.Getenv("YAK_RAG_REMOTE_EMBED") == "" {
		t.Skip("set YAK_RAG_REMOTE_EMBED=1 to call the free embedding service")
	}
	embedder, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.True(t, embedder.IsAvailable(), "free embedding service is not available")

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)

	collectionName := t.Name()
	store, err := NewSQLiteVectorStoreHNSW(
		collectionName, "", embedder.GetModelName(), embedder.GetModelDimension(), embedder, db,
		WithDisableEmbedCollectionInfo(true),
	)
	require.NoError(t, err)
	t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(db, store.collection) })
	require.Equal(t, 1024, store.collection.Dimension)
	require.Equal(t, 64, store.collection.EfSearch)
	require.Equal(t, 64, store.collection.EfConstruct)

	documents := domainCorpus()
	queries := domainQueries(documents)
	addStarted := time.Now()
	ids := make([]string, len(documents))
	for i, document := range documents {
		ids[i] = fmt.Sprintf("%s-%02d", document.topic, i)
		require.NoError(t, store.AddWithOptions(ids[i], document.text))
	}
	addElapsed := time.Since(addStarted)
	require.NoError(t, store.PersistGraph(context.Background()))

	GraphWrapperManager.RemoveCollectionFromCache(db, store.collection)
	store, err = LoadCollection(db, collectionName, WithEmbeddingClient(embedder), WithDisableEmbedCollectionInfo(true))
	require.NoError(t, err)

	listed, err := store.List()
	require.NoError(t, err)
	byID := make(map[string]scoredDoc, len(documents))
	embeddings := make(map[string][]float32, len(documents))
	for _, doc := range listed {
		if doc.ID == DocumentTypeCollectionInfo || len(doc.Embedding) != 1024 {
			continue
		}
		byID[doc.ID] = scoredDoc{id: doc.ID, topic: topicOf(doc.ID), text: doc.Content}
		embeddings[doc.ID] = doc.Embedding
	}
	require.Len(t, byID, len(documents))

	const limit = 5
	var (
		exactHits       int
		exactTotal      int
		paraphraseHits1 int
		paraphraseHits3 int
		paraphraseTotal int
		hardHits3       int
		hardTotal       int
		agreements      int
		reciprocal      float64
	)
	indexLatencies := make([]time.Duration, 0, len(queries))
	embedLatencies := make([]time.Duration, 0, len(queries))
	margins := make([]float64, 0, len(queries))

	for _, query := range queries {
		embedStarted := time.Now()
		vector, err := embedWithRetry(embedder, query.text)
		embedLatencies = append(embedLatencies, time.Since(embedStarted))
		require.NoError(t, err)
		require.Len(t, vector, 1024)

		indexStarted := time.Now()
		rawHits := store.hnsw.SearchWithDistanceAndFilter(vector, limit, func(key string, _ hnsw.Vector) bool {
			return key != DocumentTypeCollectionInfo
		})
		indexLatencies = append(indexLatencies, time.Since(indexStarted))

		hnswHits := make([]scoredDoc, 0, len(rawHits))
		for _, hit := range rawHits {
			doc, ok := byID[hit.Key]
			require.Truef(t, ok, "missing document %s", hit.Key)
			doc.score = 1 - hit.Distance
			hnswHits = append(hnswHits, doc)
		}
		exactHitsRanked := rankByCosine(vector, embeddings, byID, limit)
		require.NotEmpty(t, hnswHits)
		require.NotEmpty(t, exactHitsRanked)
		if hnswHits[0].id == exactHitsRanked[0].id {
			agreements++
		}
		require.GreaterOrEqual(t, hnswHits[0].score, hnswHits[len(hnswHits)-1].score)
		for _, hit := range hnswHits {
			require.NotEmpty(t, hit.text)
			require.GreaterOrEqual(t, hit.score, -1.0)
			require.LessOrEqual(t, hit.score, 1.0)
		}

		topicRank := rankOfTopic(hnswHits, query.topic)
		if topicRank > 0 {
			reciprocal += 1 / float64(topicRank)
		}
		margin := topicMargin(vector, embeddings, byID, query.topic)
		margins = append(margins, margin)
		t.Logf("%s topic=%s hnsw_rank=%d exact_rank=%d agree=%t margin=%.3f index=%s",
			query.kind, query.topic, topicRank, rankOfTopic(exactHitsRanked, query.topic),
			hnswHits[0].id == exactHitsRanked[0].id, margin, indexLatencies[len(indexLatencies)-1])
		t.Logf("  q: %s", query.text)
		for i, hit := range hnswHits {
			if i >= 3 {
				break
			}
			t.Logf("  #%d %s score=%.3f %s", i+1, hit.id, hit.score, trimText(hit.text, 48))
		}

		switch query.kind {
		case "exact":
			exactTotal++
			if hnswHits[0].id != "" && hnswHits[0].text == query.text && hnswHits[0].topic == query.topic && hnswHits[0].score > 0.99 {
				exactHits++
			}
		case "paraphrase":
			paraphraseTotal++
			if topicRank == 1 {
				paraphraseHits1++
			}
			if topicRank > 0 && topicRank <= 3 {
				paraphraseHits3++
			}
		case "hard":
			hardTotal++
			if topicRank > 0 && topicRank <= 3 {
				hardHits3++
			}
		}
	}

	sort.Slice(indexLatencies, func(i, j int) bool { return indexLatencies[i] < indexLatencies[j] })
	sort.Slice(embedLatencies, func(i, j int) bool { return embedLatencies[i] < embedLatencies[j] })
	sort.Float64s(margins)
	queryCount := len(queries)
	t.Logf("DOMAIN1024 docs=%d queries=%d dim=%d efSearch=%d efConstruct=%d add=%s",
		len(documents), queryCount, store.collection.Dimension, store.collection.EfSearch, store.collection.EfConstruct, addElapsed)
	t.Logf("DOMAIN1024 hnsw_exact_top1=%d/%d exact_doc_recall@1=%.3f paraphrase_recall@1=%.3f paraphrase_recall@3=%.3f hard_recall@3=%d/%d mrr=%.3f",
		agreements, queryCount,
		float64(exactHits)/float64(exactTotal),
		float64(paraphraseHits1)/float64(paraphraseTotal),
		float64(paraphraseHits3)/float64(paraphraseTotal),
		hardHits3, hardTotal,
		reciprocal/float64(queryCount))
	t.Logf("DOMAIN1024 index_p50=%s embed_p50=%s margin_p50=%.3f margin_min=%.3f",
		indexLatencies[len(indexLatencies)/2], embedLatencies[len(embedLatencies)/2], margins[len(margins)/2], margins[0])

	require.Equal(t, queryCount, agreements, "HNSW top1 disagreed with exact cosine search")
	require.Equal(t, exactTotal, exactHits, "exact document text was not rank 1")
	require.GreaterOrEqual(t, float64(paraphraseHits3)/float64(paraphraseTotal), 0.90)
	require.GreaterOrEqual(t, hardHits3, hardTotal-1)
}

func domainCorpus() []domainDoc {
	return []domainDoc{
		{"os", "虚拟内存让每个进程拥有独立地址空间，访问不在物理内存中的页面时会触发缺页并按需调入。"},
		{"os", "进程调度器依据优先级和时间片选择下一个在 CPU 上运行的线程。"},
		{"network", "TCP 通过 SYN、SYN-ACK 和 ACK 三次握手建立可靠连接，双方确认初始序列号。"},
		{"network", "DNS 把域名解析成 IP 地址，递归解析器会从根服务器开始逐级查询。"},
		{"database", "事务隔离级别决定一个事务能否看到其他事务尚未提交的写入。"},
		{"database", "B+ 树索引把键有序放在叶子节点，范围查询可以沿着叶子链表向后扫描。"},
		{"algo", "迪杰斯特拉算法要求边权非负，并从源点逐步确定到达其余顶点的最短路径。"},
		{"algo", "快速排序挑选枢轴，把数组分成较小和较大的两部分后分别递归排序。"},
		{"compiler", "词法分析把源代码切成标识符、关键字和字面量等词法单元。"},
		{"compiler", "语法分析按照文法生成抽象语法树，并检查括号与语句结构。"},
		{"crypto", "AES 是分组对称加密算法，加密和解密使用同一把密钥。"},
		{"crypto", "SHA-256 把任意长度的消息压缩成 256 位摘要，常用于完整性校验。"},
		{"web", "跨站脚本发生在页面把未经转义的用户输入当作 HTML 或脚本执行。"},
		{"web", "跨站请求伪造会诱使浏览器携带已有登录 cookie 访问受攻击的网站。"},
		{"distributed", "Raft 用领导者和日志复制让多个节点对同一条状态机日志达成一致。"},
		{"distributed", "幂等写入允许客户端重试同一个请求，而不会把副作用执行两次。"},
		{"ml", "反向传播沿着计算图把损失函数的梯度传回每一层，用来更新神经网络权重。"},
		{"ml", "过拟合时模型在训练集上准确率很高，但在验证集上明显变差。"},
		{"arch", "CPU 的 L1 缓存保存最近使用的指令和数据，容量小，延迟只有数个时钟周期。"},
		{"arch", "指令流水线把取指、译码、执行和写回重叠执行，以提高处理器吞吐。"},
		{"yaklang", "Yaklang 是面向网络安全测试的编程语言，用于编写扫描和服务验证脚本。"},
		{"yaklang", "Yakit 在桌面客户端中运行 Yaklang 脚本，把安全检测流程集中到一个界面。"},
		{"k8s", "Kubernetes NetworkPolicy 按 Pod 标签限制哪些来源可以访问某个 Service。"},
		{"k8s", "Pod 是 Kubernetes 的最小部署单元，里面可以运行一个或多个容器。"},
		{"biology", "光合作用在叶绿体中进行，光反应提供能量，卡尔文循环固定二氧化碳。"},
		{"biology", "DNA 双链依靠碱基配对相连，复制时每条链分别作为新链的模板。"},
		{"history", "丝绸之路连接中国与中亚、西亚，商队沿途运送丝绸、香料和金属器。"},
		{"history", "印刷术降低了复制书籍的成本，使同一文献能够更快流传。"},
		{"physics", "牛顿第二定律说明加速度与合外力成正比，与物体质量成反比。"},
		{"physics", "光在真空中的速度约为每秒 30 万公里，并且在相对论中保持不变。"},
		{"economics", "通货膨胀表现为物价总水平持续上升，同样数量的货币能购买的商品变少。"},
		{"economics", "供给曲线通常向右上方倾斜，价格升高时生产者愿意提供更多数量。"},
		{"law", "合同成立一般需要要约和承诺，双方就主要条款形成一致意思表示。"},
		{"law", "无罪推定要求法院依法作出有罪判决之前，被告人被视为无罪。"},
		{"medicine", "疫苗向免疫系统呈递抗原，使机体在不患病的情况下形成免疫记忆。"},
		{"medicine", "抗生素用于抑制或杀灭细菌，不能直接治疗由病毒引起的普通感冒。"},
		{"webcache", "浏览器依据 Cache-Control 和 ETag 决定是否复用之前的 HTTP 响应，从而避免重复下载静态资源。"},
	}
}

func domainQueries(docs []domainDoc) []domainQuery {
	first := map[string]string{}
	for _, doc := range docs {
		if _, ok := first[doc.topic]; !ok {
			first[doc.topic] = doc.text
		}
	}
	topics := []string{"os", "network", "database", "algo", "compiler", "crypto", "web", "distributed", "ml", "arch", "yaklang", "k8s", "biology", "history", "physics", "economics", "law", "medicine"}
	paraphrases := map[string]string{
		"os":          "为什么程序看到的内存地址可以和物理内存地址不一样？",
		"network":     "建立一条 TCP 连接时，双方需要交换哪些握手标志？",
		"database":    "如何避免一个事务读到另一个事务还没提交的修改？",
		"algo":        "所有边的权重都是正数时，应该用什么算法求单源最短路径？",
		"compiler":    "编译器怎样先把源代码拆成一个个单词？",
		"crypto":      "加密和解密共用同一把密钥的分组算法是哪一种？",
		"web":         "如果网页直接把用户输入插进 HTML，会带来什么攻击？",
		"distributed": "一组节点如何选出领导者并复制同一份操作日志？",
		"ml":          "神经网络如何把输出误差传回去调整各层参数？",
		"arch":        "离处理器核心最近、延迟最低的缓存通常叫什么？",
		"yaklang":     "编写网络安全扫描脚本时可以使用哪门专用语言？",
		"k8s":         "在 Kubernetes 里怎样按标签限制 Pod 之间的网络访问？",
		"biology":     "植物在哪个细胞器里把二氧化碳合成有机物？",
		"history":     "古代连接中国和西亚、运送丝绸的商路叫什么？",
		"physics":     "力、质量和加速度之间的基本关系是什么？",
		"economics":   "为什么同样多的钱后来买到的东西变少了？",
		"law":         "一份合同通常要经过哪两个意思表示才会成立？",
		"medicine":    "接种疫苗之后，免疫系统为什么能更快认出同一种病原体？",
	}
	queries := make([]domainQuery, 0, len(topics)*2+2)
	for _, topic := range topics {
		queries = append(queries, domainQuery{topic: topic, text: first[topic], kind: "exact"})
		queries = append(queries, domainQuery{topic: topic, text: paraphrases[topic], kind: "paraphrase"})
	}
	queries = append(queries,
		domainQuery{topic: "webcache", text: "浏览器如何判断静态文件没有变化、从而不用重新下载？", kind: "hard"},
		domainQuery{topic: "arch", text: "处理器内部最小最快的数据缓存有什么特点？", kind: "hard"},
	)
	return queries
}

func embedWithRetry(embedder *AIBalanceFreeEmbedding, text string) ([]float32, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		vector, err := embedder.Embedding(text)
		if err == nil && len(vector) == 1024 {
			return vector, nil
		}
		last = err
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("embedding dimension is not 1024")
	}
	return nil, last
}

func rankByCosine(query []float32, embeddings map[string][]float32, docs map[string]scoredDoc, limit int) []scoredDoc {
	ranked := make([]scoredDoc, 0, len(embeddings))
	queryVector := func() []float32 { return query }
	for id, embedding := range embeddings {
		vector := embedding
		doc := docs[id]
		doc.score = 1 - hnsw.CosineDistance(queryVector, func() []float32 { return vector })
		ranked = append(ranked, doc)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].id < ranked[j].id
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func topicMargin(query []float32, embeddings map[string][]float32, docs map[string]scoredDoc, topic string) float64 {
	queryVector := func() []float32 { return query }
	bestTopic := -2.0
	bestOther := -2.0
	for id, embedding := range embeddings {
		vector := embedding
		score := 1 - hnsw.CosineDistance(queryVector, func() []float32 { return vector })
		if docs[id].topic == topic {
			if score > bestTopic {
				bestTopic = score
			}
			continue
		}
		if score > bestOther {
			bestOther = score
		}
	}
	return bestTopic - bestOther
}

func rankOfTopic(hits []scoredDoc, topic string) int {
	for i, hit := range hits {
		if hit.topic == topic {
			return i + 1
		}
	}
	return 0
}

func topicOf(id string) string {
	if dash := strings.LastIndex(id, "-"); dash > 0 {
		return id[:dash]
	}
	return id
}

func trimText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}
