package aiknowledge

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func decode(input map[string]any, target any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Table fixtures describe invalid combinations compactly; the actual call still
// crosses the public positional-argument/functional-option boundary.
func manageRequestForTest(input map[string]any, opts ...Option) (map[string]any, error) {
	var req manageRequest
	if err := decode(input, &req); err != nil {
		return nil, err
	}
	options := append([]Option{}, opts...)
	if req.Limit != 0 {
		options = append(options, WithLimit(req.Limit))
	}
	options = append(options, WithOffset(req.Offset), WithKeyword(req.Keyword))
	m := req.Metadata
	if m.Description != nil {
		options = append(options, WithDescription(*m.Description))
	}
	if m.Type != nil {
		options = append(options, WithKnowledgeBaseType(*m.Type))
	}
	if m.Tags != nil {
		options = append(options, WithTags((*m.Tags)...))
	}
	if m.IsDefault != nil {
		options = append(options, WithIsDefault(*m.IsDefault))
	}
	i := req.IndexOptions
	if i.EmbeddingModel != "" {
		options = append(options, WithEmbeddingModel(i.EmbeddingModel))
	}
	if i.Dimension != 0 {
		options = append(options, WithDimension(i.Dimension))
	}
	if i.M != 0 {
		options = append(options, WithHNSWM(i.M))
	}
	if i.Ml != 0 {
		options = append(options, WithHNSWML(i.Ml))
	}
	if i.EfSearch != 0 {
		options = append(options, WithHNSWEFSearch(i.EfSearch))
	}
	if i.EfConstruct != 0 {
		options = append(options, WithHNSWEFConstruct(i.EfConstruct))
	}
	if req.Index != nil {
		options = append(options, WithIndex(*req.Index))
	}
	title, content := "", ""
	if e := req.Entry; e != nil {
		title, content = e.Title, e.Content
		options = append(options, WithEntrySummary(e.Summary), WithEntryType(e.Type), WithEntryKeywords(e.Keywords...), WithPotentialQuestions(e.PotentialQuestions...))
	}
	b := req.BuildOptions
	options = append(options, WithBuildMode(b.Mode), WithChunkSize(b.ChunkSize), WithConcurrency(b.Concurrency), WithExtraPrompt(b.ExtraPrompt))
	switch req.Op {
	case "list":
		return List(options...)
	case "create":
		return Add(req.Name, options...)
	case "get":
		return Get(req.Name, options...)
	case "update":
		return Update(req.Name, options...)
	case "delete":
		return Delete(req.Name, options...)
	case "add_entry":
		return AddEntry(req.Name, title, content, options...)
	case "update_entry":
		return UpdateEntry(req.Name, req.EntryID, title, content, options...)
	case "delete_entry":
		return DeleteEntry(req.Name, req.EntryID, options...)
	case "rebuild_index":
		return RebuildIndex(req.Name, options...)
	case "build":
		if req.SourceFile != "" && req.Content != "" {
			return nil, fmt.Errorf("build requires exactly one of source_file or content")
		}
		if req.SourceFile != "" {
			return BuildFromFile(req.Name, req.SourceFile, options...)
		}
		return Build(req.Name, req.Content, options...)
	default:
		return nil, fmt.Errorf("unknown operation %q", req.Op)
	}
}

func queryRequestForTest(input map[string]any, opts ...Option) (*QueryResult, error) {
	var req queryRequest
	if err := decode(input, &req); err != nil {
		return nil, err
	}
	var scope any = req.Name
	if len(req.Names) > 0 {
		scope = req.Names
	}
	options := append(append([]Option{}, opts...), func(c *config) { c.query = req })
	return Query(scope, req.Query, req.TokenLimit, options...)
}
