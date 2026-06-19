package tools

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"path"
)

var homeDir string

func init() {
	homeDir = utils.GetHomeDirDefault(".")
}

func BinaryLocations(binaryName ...string) []string {
	//return []string{
	//	"./subfinder",
	//	path.Join(home, "subfinder"),
	//	path.Join("/usr/local/bin", "subfinder"),
	//	path.Join("/usr/bin/", "subfinder"),
	//}
	var s []string
	for _, binary := range binaryName {
		s = append(s,
			path.Join(homeDir, binary),
			path.Join("/usr/local/bin", binary),
			path.Join("/usr/bin/", binary),
			path.Join("/", binary),
			path.Join(".", binary),
			path.Join(homeDir, "Project/tmp", binary),
		)
	}
	s = utils.RemoveRepeatStringSlice(s)
	return s
}

func ResourceLocations(resResources ...string) []string {
	var s []string
	for _, dirName := range resResources {
		s = append(s,
			path.Join(homeDir, dirName),
			path.Join(".", dirName),
		)
	}
	s = utils.RemoveRepeatStringSlice(s)
	return s
}

var Exports = map[string]interface{}{

	// 子域名扫描
	//"ScanSubDomain": palmscanlib.ScanSubDomainQuick,
	//"NewSubFinder": func() (*SubFinderInstance, error) {
	//	return NewSubFinderInstance()
	//},
	"NewPocInvoker": NewPocInvoker,
	"NewBruteUtil":  NewBruteUtil,
}

// NewBruteUtil 根据指定的服务类型创建一个多目标爆破工具(BruteUtil)
// 在 yak 中通过 tools.NewBruteUtil 调用，服务类型如 "ssh"、"redis"、"mysql" 等
// 参数:
//   - t: 爆破目标的服务类型名称
//
// 返回值:
//   - 爆破工具对象，可用于对多个目标执行口令爆破
//   - 错误信息，类型不支持或创建失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：创建 ssh 爆破工具
// util = tools.NewBruteUtil("ssh")~
// println(util != nil)
// ```
func NewBruteUtil(t string) (*bruteutils.BruteUtil, error) {
	res, err := bruteutils.GetBruteFuncByType(t)
	if err != nil {
		return nil, err
	}
	ut, err := bruteutils.NewMultiTargetBruteUtil(256, 1, 5, res)
	if err != nil {
		return nil, utils.Errorf("create brute utils failed: %s", err)
	}
	return ut, nil
}
