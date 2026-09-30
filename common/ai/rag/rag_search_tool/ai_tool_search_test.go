package rag_search_tool

import (
	"errors"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestLazyRAGSearcherInitializesOnSearch(t *testing.T) {
	created, searched := 0, 0
	search := newLazyRAGSearcher(func() (searchtools.AISearcher[*schema.AIForge], error) {
		created++
		return func(query string, list []*schema.AIForge) ([]*schema.AIForge, error) {
			searched++
			require.Equal(t, "local query", query)
			return list, nil
		}, nil
	})
	require.Zero(t, created)
	for i := 0; i < 2; i++ {
		tools := []*schema.AIForge{{ForgeName: "local forge"}}
		got, err := search("local query", tools)
		require.NoError(t, err)
		require.Equal(t, tools, got)
	}
	require.Equal(t, 1, created)
	require.Equal(t, 2, searched)
}
func TestLazyRAGSearcherPreservesInitializationError(t *testing.T) {
	expected := errors.New("no local index")
	search := newLazyRAGSearcher(func() (searchtools.AISearcher[*schema.AIForge], error) { return nil, expected })
	for i := 0; i < 2; i++ {
		_, err := search("query", nil)
		require.ErrorIs(t, err, expected)
	}
}
