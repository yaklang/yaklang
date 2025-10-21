package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"testing"
)

func TestExportTemp(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.ExportKnowledgeBase(context.Background(), &ypb.ExportKnowledgeBaseRequest{
		KnowledgeBaseId: 1,
		TargetPath:      "/Users/rookie/Documents/test-data/yaklang.rag",
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		msg, err := result.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		spew.Dump(msg)
	}
}
