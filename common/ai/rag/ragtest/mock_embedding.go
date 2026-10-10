// Package ragtest contains helpers shared by RAG tests.
package ragtest

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/utils"
)

// MockEmbeddingClient adds synthetic test document generation to the offline
// embedding client. Production users only need the embedding implementation.
type MockEmbeddingClient struct {
	*vectorstore.MockEmbeddingClient
	vocabulary []string
	dimension  int
}

func NewDefaultMockEmbedding() *MockEmbeddingClient {
	vocabulary := make([]string, 0, len(vectorstore.Vocabulary1024))
	seen := make(map[string]bool)
	for _, word := range vectorstore.Vocabulary1024 {
		if !seen[word] {
			seen[word] = true
			vocabulary = append(vocabulary, word)
		}
	}
	sort.Strings(vocabulary)
	return &MockEmbeddingClient{
		MockEmbeddingClient: vectorstore.NewDefaultMockEmbedding(),
		vocabulary:          vocabulary,
		dimension:           len(vocabulary),
	}
}

// GenerateRandomText 从词典中随机选择词汇来生成一段文本。
func (c *MockEmbeddingClient) GenerateRandomText(wordCount int) string {
	return strings.Join(c.GenerateRandomWord(wordCount), " ")
}

func (c *MockEmbeddingClient) GenerateRandomWord(wordCount int) []string {
	if wordCount <= 0 {
		return nil
	}
	// Clock resolution can repeat across rapid calls, especially on Windows.
	// Draw independent seeds from the concurrency-safe process RNG instead.
	source := rand.NewSource(rand.Int63())
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

	// Clock resolution can repeat across rapid calls, especially on Windows.
	// Draw independent seeds from the concurrency-safe process RNG instead.
	source := rand.NewSource(rand.Int63())
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
