package yak

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestYakAIReducerCallbacks(t *testing.T) {
	for _, name := range []string{"callback", "reducerCallback"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			engine := NewScriptEngine(1)
			_, err := engine.ExecuteExWithContext(ctx, fmt.Sprintf(`
source = "first line\n第二行\nlast line"
combined = ""
count = 0
reducer = aireducer.NewReducerFromString(source,
    aireducer.chunkSize(8),
    aireducer.memory({"ignored": true}),
    aireducer.%s(func(config, memory, chunk) {
        assert config.ChunkSize == 8
        assert memory == nil
        combined += string(chunk.Data())
        count++
        return nil
    }))~
reducer.Run()~
assert combined == source
assert count > 1

combined = ""
aireducer.String("abcdefghijkl", func(chunk) {
    combined += string(chunk.Data())
}, aireducer.chunkSize(4), aireducer.memory(nil))~
assert combined == "abcdefghijkl"
`, name), nil)
			require.NoError(t, err)
		})
	}
}
