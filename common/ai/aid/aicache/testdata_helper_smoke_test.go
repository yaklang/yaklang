package aicache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixtureLoader_Smoke 验证 loadFixtureRawPrompt 能成功解析
// dump 元数据并截出与 DeclaredBytes 字节数一致的原 prompt 段。
// 关键词: aicache, fixture loader smoke
func TestFixtureLoader_Smoke(t *testing.T) {
	cases := []struct {
		file   string
		chunks int
	}{
		{"000001.txt", 3},
		{"000005.txt", 4},
		{"000045.txt", 1},
		{"000060.txt", 4},
	}
	for _, c := range cases {
		c := c
		t.Run(c.file, func(t *testing.T) {
			meta := loadFixtureRawPrompt(t, c.file)
			require.NotNil(t, meta)
			assert.Equal(t, c.chunks, meta.DeclaredChunks, "DeclaredChunks mismatch for %s", c.file)
			assert.Len(t, meta.Sections, c.chunks, "Sections length mismatch for %s", c.file)
			assert.Equal(t, meta.DeclaredBytes, len(meta.Raw), "Raw byte length should match DeclaredBytes")

			split := Split(meta.Raw)
			assert.Equal(t, c.chunks, len(split.Chunks), "Split chunk count should match dump declaration")
			for i, sec := range meta.Sections {
				assert.Equal(t, sec.Section, split.Chunks[i].Section, "section[%d] name mismatch", i)
				assert.Equal(t, sec.Bytes, split.Chunks[i].Bytes, "section[%d] bytes mismatch", i)
			}
		})
	}
}
