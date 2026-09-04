package randomforest

import (
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
