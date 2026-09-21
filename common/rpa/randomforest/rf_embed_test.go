package randomforest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLoadEmbeddedModel 校验内置在二进制里的 rf.model.gz 能真正解出可用的森林：
// 之前这里读的是一个写死的开发机绝对路径，除作者机器外都加载不到模型。
func TestLoadEmbeddedModel(t *testing.T) {
	sys := &UrlDetectSys{}
	assert.NoError(t, sys.LoadEmbeddedModel())
	assert.Contains(t, []string{"0", "1"}, sys.PredictX("http://example.com/index.php?a=select+1"))
}

func TestModelRoundTrip(t *testing.T) {
	sys := &UrlDetectSys{}
	assert.NoError(t, sys.LoadEmbeddedModel())
	path := t.TempDir() + "/model.json"
	assert.NoError(t, sys.DumpModel(path))
	loaded := &UrlDetectSys{}
	assert.NoError(t, loaded.LoadModel(path))
	assert.Equal(t, sys.model, loaded.model)
	// Overwriting a larger file must truncate the previous JSON.
	sys.model.Trees = sys.model.Trees[:1]
	assert.NoError(t, sys.DumpModel(path))
	assert.NoError(t, loaded.LoadModel(path))
	assert.Equal(t, sys.model, loaded.model)
	assert.Error(t, loaded.LoadModel(path+".missing"))
	assert.NoError(t, os.WriteFile(path, []byte("{broken"), 0600))
	assert.Error(t, loaded.LoadModel(path))
	assert.Error(t, sys.DumpModel(path+"/invalid"))
	assert.Error(t, (&UrlDetectSys{}).DumpModel(path))
}
