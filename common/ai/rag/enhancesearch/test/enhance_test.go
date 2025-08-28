package enhancesearch

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/yak/depinjector/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestRewriteQuerySearch(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	enhancesearch.Simpleliteforge = aiforge.SimpleAiForgeIns
	query := "什么是yaklang"
	enhance, err := enhancesearch.HypotheticalAnswer(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	println(enhance)
}

func TestSplitQuerySearch(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	enhancesearch.Simpleliteforge = aiforge.SimpleAiForgeIns
	query := "什么是yaklang"
	enhance, err := enhancesearch.SplitQuery(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(enhance)
}

func TestGeneralizeQuerySearch(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	enhancesearch.Simpleliteforge = aiforge.SimpleAiForgeIns
	query := "什么是yaklang"
	enhance, err := enhancesearch.GeneralizeQuery(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	println(enhance)
}
