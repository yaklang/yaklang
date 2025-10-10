package enhancesearch

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
)

type MockSearchHandler struct {
	ExtractKeywordsFunc    func(ctx context.Context, query string) ([]string, error)
	HypotheticalAnswerFunc func(ctx context.Context, query string) (string, error)
	GeneralizeQueryFunc    func(ctx context.Context, query string) ([]string, error)
	SplitQueryFunc         func(ctx context.Context, query string) ([]string, error)
}

func NewMockSearchHandler() *MockSearchHandler {
	return &MockSearchHandler{}
}

func (h *MockSearchHandler) SetExtractKeywordsFunc(f func(ctx context.Context, query string) ([]string, error)) {
	h.ExtractKeywordsFunc = f
}

func (h *MockSearchHandler) SetHypotheticalAnswerFunc(f func(ctx context.Context, query string) (string, error)) {
	h.HypotheticalAnswerFunc = f
}

func (h *MockSearchHandler) SetGeneralizeQueryFunc(f func(ctx context.Context, query string) ([]string, error)) {
	h.GeneralizeQueryFunc = f
}

func (h *MockSearchHandler) SetSplitQueryFunc(f func(ctx context.Context, query string) ([]string, error)) {
	h.SplitQueryFunc = f
}

func (h *MockSearchHandler) ExtractKeywords(ctx context.Context, query string) ([]string, error) {
	if h.ExtractKeywordsFunc != nil {
		return h.ExtractKeywordsFunc(ctx, query)
	}
	return nil, utils.Errorf("mock func not implemented")
}

func (h *MockSearchHandler) HypotheticalAnswer(ctx context.Context, query string) (string, error) {
	if h.HypotheticalAnswerFunc != nil {
		return h.HypotheticalAnswerFunc(ctx, query)
	}
	return "", utils.Errorf("mock func not implemented")
}

func (h *MockSearchHandler) GeneralizeQuery(ctx context.Context, query string) ([]string, error) {
	if h.GeneralizeQueryFunc != nil {
		return h.GeneralizeQueryFunc(ctx, query)
	}
	return nil, utils.Errorf("mock func not implemented")
}

func (h *MockSearchHandler) SplitQuery(ctx context.Context, query string) ([]string, error) {
	if h.SplitQueryFunc != nil {
		return h.SplitQueryFunc(ctx, query)
	}
	return nil, utils.Errorf("mock func not implemented")
}
