//go:build gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed buildin.tar.gz
var ruleFS embed.FS

var (
	initOnce   sync.Once
	initErr    error
	initNotify func(process float64, ruleName string)
)

func InitEmbedFS() {
	// 延迟初始化，不在 init() 中执行，避免启动时阻塞
	// 将在首次调用 GetRuleFileSystem() 时执行
}

func init() {
	// 不在 init() 中执行解压，避免启动时阻塞
}

// InitEmbedFSWithNotify 带进度通知的初始化（用于首次使用时）
func InitEmbedFSWithNotify(notify func(process float64, ruleName string)) {
	initNotify = notify
	GetRuleFileSystem() // 触发初始化
}

// getRuleFS returns the ruleFS embed.FS
func getRuleFS() interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
} {
	return ruleFS
}

// GetRuleFileSystem 返回规则文件系统实例（gzip 版本）
// 如果尚未初始化，会在首次调用时进行延迟初始化（包括解压 tar.gz）
func GetRuleFileSystem() filesys_interface.FileSystem {
	initOnce.Do(func() {
		if initNotify != nil {
			initNotify(0, "正在解压规则文件...")
		}
		var fs *gzip_embed.PreprocessingEmbed
		fs, initErr = gzip_embed.NewPreprocessingEmbed(&ruleFS, "buildin.tar.gz", true)
		if initErr != nil {
			log.Errorf("init embed failed: %v", initErr)
			fs = gzip_embed.NewEmptyPreprocessingEmbed()
		}
		ruleFSWithHash = fs
		if initNotify != nil {
			initNotify(0.05, "规则文件解压完成")
		}
	})
	// ruleFSWithHash 在 gzip 版本中已经是 PreprocessingEmbed，实现了 FileSystem 接口
	return ruleFSWithHash.(filesys_interface.FileSystem)
}