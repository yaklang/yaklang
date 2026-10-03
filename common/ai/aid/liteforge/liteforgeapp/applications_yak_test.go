package liteforgeapp_test

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/yak"
)

//go:embed smoke.yak
var protocolSmoke string

//go:embed smoke_applications.yak
var applicationsSmoke string

func TestYakMigrationSmoke(t *testing.T) {
	previousMockMode := vectorstore.IsMockMode
	t.Cleanup(func() { vectorstore.IsMockMode = previousMockMode })
	for _, script := range []struct{ name, source string }{
		{"protocols", protocolSmoke},
		{"applications", applicationsSmoke},
	} {
		t.Run(script.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := yak.NewScriptEngine(1).ExecuteExWithContext(ctx, script.source, nil)
			require.NoError(t, err)
		})
	}
}
