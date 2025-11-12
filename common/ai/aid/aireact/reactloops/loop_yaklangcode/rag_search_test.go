package loop_yaklangcode

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestVerifyYaklangAIKBRag(t *testing.T) {
	p := `/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag`
	docSearcherByRag, err := createDocumentSearcherByRag(consts.GetGormProfileDatabase(), defaultYaklangAIKBRagCollectionName, p)
	if err != nil {
		log.Errorf("failed to create document searcher by rag: %v", err)
		docSearcherByRag = nil // 明确设置为 nil，语义搜索将不可用
	}
	_ = docSearcherByRag
	results, err := docSearcherByRag.QueryTopN("TOTP如何使用？", 10)
	if err != nil {
		t.Fatalf("failed to query top N: %v", err)
	}
	spew.Dump(results)
}
