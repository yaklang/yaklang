package reactloops

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestActionStreamHandler(t *testing.T) {
	var count = new(int64)
	var keys = make(map[string]struct{})
	var m = new(sync.Mutex)
	action, _ := aicommon.ExtractActionFromStream(
		context.Background(),
		bytes.NewBufferString(`{"@action": "object", "key": "value"}`),
		"object",
		aicommon.WithActionFieldStreamHandler([]string{
			"key", "key1", "a",
		}, func(key string, r io.Reader) {
			m.Lock()
			defer m.Unlock()
			keys[key] = struct{}{}
			atomic.AddInt64(count, 1)
			if key == "key" {
				value, _ := io.ReadAll(r)
				assert.Equal(t, `"value"`, string(value))
			}
		}),
	)

	action.GetString("key")
	if _, ok := keys["key"]; !ok {
		t.Errorf("expected key 'key' to be handled")
	}

	action.WaitStream(context.Background())

	spew.Dump(action.GetString("key1"))
	spew.Dump(action.GetString("a"))
}
