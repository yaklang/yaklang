package enhancesearch

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	// import aiforge to register liteforge callback via init()
	_ "github.com/yaklang/yaklang/common/ai/aid"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
	_ "github.com/yaklang/yaklang/common/aiforge"
)

func TestRewriteQuerySearch(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	query := "什么是yaklang"
	handler := enhancesearch.NewDefaultSearchHandler()

	enhance, err := handler.HypotheticalAnswer(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	println(enhance)
}

func TestSplitQuerySearch(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	query := "什么是yaklang"
	handler := enhancesearch.NewDefaultSearchHandler()
	enhance, err := handler.SplitQuery(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(enhance)
}

func TestGeneralizeQuerySearch(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	query := "什么是yaklang"
	handler := enhancesearch.NewDefaultSearchHandler()
	enhance, err := handler.GeneralizeQuery(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	println(enhance)
}
