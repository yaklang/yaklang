package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

// TestHashComparison 测试 gzip_embed 和标准 embed.FS 生成的哈希值是否相同
func TestHashComparison(t *testing.T) {
	// 1. 获取 gzip_embed 的哈希值
	gzipHash, err := FS.GetHash()
	if err != nil {
		t.Fatalf("Failed to get gzip_embed hash: %v", err)
	}
	t.Logf("gzip_embed hash: %s", gzipHash)

	// 2. 获取标准 embed.FS 的哈希值
	standardHash, err := filesys.CreateEmbedFSHash(standardFS)
	if err != nil {
		t.Fatalf("Failed to get standard embed.FS hash: %v", err)
	}
	t.Logf("standard embed.FS hash: %s", standardHash)

	// 3. 比较两个哈希值
	assert.Equal(t, standardHash, gzipHash,
		"Hash from gzip_embed and standard embed.FS should be the same")

	// 4. 验证哈希值不为空且长度正确（SHA256 = 64 个十六进制字符）
	assert.NotEmpty(t, gzipHash, "gzip_embed hash should not be empty")
	assert.NotEmpty(t, standardHash, "standard embed.FS hash should not be empty")
	assert.Equal(t, 64, len(gzipHash), "SHA256 hash should be 64 characters")
	assert.Equal(t, 64, len(standardHash), "SHA256 hash should be 64 characters")
}

// TestHashStability 测试哈希值的稳定性
func TestHashStability(t *testing.T) {
	// 多次计算 gzip_embed 的哈希值，应该保持一致
	hash1, err := FS.GetHash()
	assert.NoError(t, err)

	// 清除缓存后重新计算
	FS.InvalidateHash()
	hash2, err := FS.GetHash()
	assert.NoError(t, err)

	assert.Equal(t, hash1, hash2, "Hash should be stable across multiple calculations")

	// 多次计算标准 embed.FS 的哈希值，应该保持一致
	stdHash1, err := filesys.CreateEmbedFSHash(standardFS)
	assert.NoError(t, err)

	stdHash2, err := filesys.CreateEmbedFSHash(standardFS)
	assert.NoError(t, err)

	assert.Equal(t, stdHash1, stdHash2, "Standard embed.FS hash should be stable")
}

// TestHashConsistency 测试不同文件系统实现的一致性
func TestHashConsistency(t *testing.T) {
	// 创建两个不同的 gzip_embed 实例（一个缓存，一个不缓存）
	cachedFS := FS

	// 不缓存的实例
	notCachedFS, err := gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", false)
	if err != nil {
		t.Fatalf("Failed to create non-cached FS: %v", err)
	}

	// 两种实例的哈希应该相同
	cachedHash, err := cachedFS.GetHash()
	assert.NoError(t, err)

	notCachedHash, err := notCachedFS.GetHash()
	assert.NoError(t, err)

	assert.Equal(t, cachedHash, notCachedHash,
		"Cached and non-cached FS should produce the same hash")

	// 与标准 embed.FS 的哈希也应该相同
	standardHash, err := filesys.CreateEmbedFSHash(standardFS)
	assert.NoError(t, err)

	assert.Equal(t, standardHash, cachedHash,
		"All implementations should produce the same hash")
}
