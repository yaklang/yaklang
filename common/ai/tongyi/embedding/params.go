package embedding

const (
	embeddingURL = "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"
)

type ModelEmbedding = string

const (
	TextEmbeddingV1      = "text-embedding-v1"
	TextEmbeddingAsyncV1 = "text-embedding-async-v1"
	TextEmbeddingV2      = "text-embedding-v2"
	TextEmbeddingAsyncV2 = "text-embedding-async-v2"
)

type TextType = string

const (
	TypeQuery    TextType = "query"
	TypeDocument TextType = "document"
)
