package aiknowledge

func WithLimit(limit int) Option {
	return func(c *config) { c.manage.Limit, c.query.Limit = limit, limit }
}
func WithOffset(offset int) Option {
	return func(c *config) { c.manage.Offset, c.query.Offset = offset, offset }
}
func WithKeyword(keyword string) Option { return func(c *config) { c.manage.Keyword = keyword } }
func WithMode(mode string) Option       { return func(c *config) { c.query.Mode = mode } }
func WithAll(all bool) Option           { return func(c *config) { c.query.All = all } }
func WithPerEntryTokenLimit(limit int) Option {
	return func(c *config) { c.query.PerEntryTokenLimit = limit }
}
func WithContentOffset(offset int) Option { return func(c *config) { c.query.ContentOffset = offset } }
func WithMinScore(score float64) Option   { return func(c *config) { c.query.MinScore = &score } }
func WithEntryID(id string) Option {
	return func(c *config) { c.manage.EntryID, c.query.EntryID = id, id }
}
func WithDescription(description string) Option {
	return func(c *config) { c.manage.Metadata.Description = &description }
}
func WithKnowledgeBaseType(kind string) Option {
	return func(c *config) { c.manage.Metadata.Type = &kind }
}
func WithTags(tags ...string) Option {
	return func(c *config) { values := append([]string{}, tags...); c.manage.Metadata.Tags = &values }
}
func WithIsDefault(value bool) Option {
	return func(c *config) { c.manage.Metadata.IsDefault = &value }
}
func WithEmbeddingModel(model string) Option {
	return func(c *config) { c.manage.IndexOptions.EmbeddingModel = model }
}
func WithDimension(dimension int) Option {
	return func(c *config) { c.manage.IndexOptions.Dimension = dimension }
}
func WithHNSWM(m int) Option       { return func(c *config) { c.manage.IndexOptions.M = m } }
func WithHNSWML(ml float64) Option { return func(c *config) { c.manage.IndexOptions.Ml = ml } }
func WithHNSWEFSearch(value int) Option {
	return func(c *config) { c.manage.IndexOptions.EfSearch = value }
}
func WithHNSWEFConstruct(value int) Option {
	return func(c *config) { c.manage.IndexOptions.EfConstruct = value }
}
func WithIndex(index bool) Option { return func(c *config) { c.manage.Index = &index } }
func (c *config) entry() *entryInput {
	if c.manage.Entry == nil {
		c.manage.Entry = &entryInput{}
	}
	return c.manage.Entry
}
func WithEntrySummary(summary string) Option { return func(c *config) { c.entry().Summary = summary } }
func WithEntryType(kind string) Option       { return func(c *config) { c.entry().Type = kind } }
func WithEntryKeywords(words ...string) Option {
	return func(c *config) { c.entry().Keywords = append([]string{}, words...) }
}
func WithPotentialQuestions(questions ...string) Option {
	return func(c *config) { c.entry().PotentialQuestions = append([]string{}, questions...) }
}
func WithBuildMode(mode string) Option { return func(c *config) { c.manage.BuildOptions.Mode = mode } }
func WithChunkSize(size int) Option {
	return func(c *config) { c.manage.BuildOptions.ChunkSize = size }
}
func WithConcurrency(concurrency int) Option {
	return func(c *config) { c.manage.BuildOptions.Concurrency = concurrency }
}
func WithExtraPrompt(prompt string) Option {
	return func(c *config) { c.manage.BuildOptions.ExtraPrompt = prompt }
}
func WithOmniMode(mode string) Option {
	return func(c *config) { c.manage.BuildOptions.OmniMode = mode }
}
func WithOmniModel(model string) Option {
	return func(c *config) { c.manage.BuildOptions.OmniModel = model }
}
func WithOmniSegmentSeconds(seconds float64) Option {
	return func(c *config) { c.manage.BuildOptions.OmniSegmentSeconds = seconds }
}
func WithOmniPayloadBytes(size int64) Option {
	return func(c *config) { c.manage.BuildOptions.OmniPayloadBytes = size }
}

var Exports = map[string]any{
	"List": List, "Add": Add, "Get": Get, "Update": Update, "Delete": Delete,
	"Build": Build, "BuildFromFile": BuildFromFile, "RebuildIndex": RebuildIndex,
	"AddEntry": AddEntry, "UpdateEntry": UpdateEntry, "DeleteEntry": DeleteEntry, "Query": Query,
	"db": WithDatabase, "context": WithContext, "aiService": WithAIService,
	"limit": WithLimit, "offset": WithOffset, "keyword": WithKeyword,
	"mode": WithMode, "all": WithAll, "perEntryTokenLimit": WithPerEntryTokenLimit,
	"contentOffset": WithContentOffset, "minScore": WithMinScore, "entryID": WithEntryID,
	"description": WithDescription, "type": WithKnowledgeBaseType, "tags": WithTags, "isDefault": WithIsDefault,
	"embeddingModel": WithEmbeddingModel, "dimension": WithDimension,
	"hnswM": WithHNSWM, "hnswML": WithHNSWML, "hnswEFSearch": WithHNSWEFSearch, "hnswEFConstruct": WithHNSWEFConstruct,
	"index": WithIndex, "entrySummary": WithEntrySummary, "entryType": WithEntryType,
	"entryKeywords": WithEntryKeywords, "potentialQuestions": WithPotentialQuestions,
	"buildMode": WithBuildMode,
	"chunkSize": WithChunkSize, "concurrency": WithConcurrency, "extraPrompt": WithExtraPrompt,
	"omniMode": WithOmniMode, "omniModel": WithOmniModel,
	"omniSegmentSeconds": WithOmniSegmentSeconds, "omniPayloadBytes": WithOmniPayloadBytes,
}
