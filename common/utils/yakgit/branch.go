package yakgit

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/utils"
)

// GetAllBranches 获取本地仓库中的所有引用名（导出名为 git.Branch）
// 参数:
//   - repos: 本地仓库路径
//
// 返回值:
//   - 引用名列表
//   - 错误信息
//
// Example:
// ```
// // 列出仓库的引用（示意性示例，需替换为真实仓库路径）
// branches = git.Branch("/path/to/repo")~
// dump(branches)
// ```
func GetAllBranches(repos string) ([]string, error) {
	rep, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return nil, err
	}
	iter, err := rep.References()
	if err != nil {
		return nil, utils.Errorf("fetch refs iters failed: %s", err)
	}
	var i []string
	iter.ForEach(func(ref *plumbing.Reference) error {
		n := ref.Name()
		if n.IsTag() || n.IsBranch() {
			return nil
		}
		shortName := n.Short()
		i = append(i, shortName)
		return nil
	})
	return i, nil
}
