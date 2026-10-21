package imagecache

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
)

// Open 按规则顺序和编译选项复用本机镜像。
// 文件缺失、规则变化或损坏时由 LoadOrCompile 重新编译并原子替换。
// 缓存目录不可写时退回内存编译，扫描结果与直接 Compile 相同。
func Open(group string, patterns []minirehs.Pattern, opts ...minirehs.Option) (minirehs.Database, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("minirehs: no patterns")
	}
	path, err := patternFile(group, patterns)
	if err == nil {
		db, loadErr := minirehs.LoadOrCompile(path, patterns, opts...)
		if loadErr == nil {
			return db, nil
		}
		log.Warnf("minirehs image cache unavailable, compiling in memory: group=%s path=%s err=%v", group, path, loadErr)
	}
	return minirehs.Compile(patterns, opts...)
}

func patternFile(group string, patterns []minirehs.Pattern) (string, error) {
	if group == "" {
		return "", fmt.Errorf("minirehs: empty image group")
	}
	root := cacheRoot()
	if root == "" {
		return "", fmt.Errorf("minirehs: empty image cache root")
	}
	sum := patternSum(patterns)
	return filepath.Join(root, group, fmt.Sprintf("%016x.rehs", sum)), nil
}

func cacheRoot() string {
	if dir := os.Getenv("YAK_MINIREHS_CACHE"); dir != "" {
		return dir
	}
	return filepath.Join(consts.GetDefaultYakitBaseDir(), "minirehs", "fp")
}

func patternSum(patterns []minirehs.Pattern) uint64 {
	h := fnv.New64()
	var buf [12]byte
	for _, p := range patterns {
		binary.LittleEndian.PutUint32(buf[0:4], uint32(p.ID))
		binary.LittleEndian.PutUint32(buf[4:8], uint32(p.Flags))
		binary.LittleEndian.PutUint32(buf[8:12], uint32(len(p.Expr)))
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte(p.Expr))
	}
	return h.Sum64()
}
