package tools

import (
	"path"
	"yaklang/common/utils"
	"yaklang/common/utils/bruteutils"
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
	"NewPocInvoker": func() (*PocInvoker, error) {
		return NewPocInvoker()
	},
	"NewBruteUtil": func(t string) (*bruteutils.BruteUtil, error) {
		res, err := bruteutils.GetBruteFuncByType(t)
		if err != nil {
			return nil, err
		}
		ut, err := bruteutils.NewMultiTargetBruteUtil(256, 1, 5, res)
		if err != nil {
			return nil, utils.Errorf("create brute utils failed: %s", err)
		}
		return ut, nil
	},
}
