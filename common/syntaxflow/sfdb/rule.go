package sfdb

import (
	"io/fs"
	"strconv"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// 已弃用
func LoadFileSystem(s *schema.SyntaxFlowRule, system fi.FileSystem) error {
	f := make(map[string]string)
	filesys.Recursive(".", filesys.WithFileSystem(system), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		raw, err := system.ReadFile(s)
		if err != nil {
			return nil
		}
		f[s] = strconv.Quote(string(raw))
		return nil
	}))
	// raw, err := json.Marshal(f)
	// if err != nil {
	// 	return utils.Wrapf(err, `failed to marshal file system`)
	// }

	// s.TypicalHitFileSystem, _ = utils.GzipCompress(raw)
	return nil
}

// 已弃用
func BuildFileSystem(s *schema.SyntaxFlowRule) (fi.FileSystem, error) {
	f := make(map[string]string)
	// raw, _ := utils.GzipDeCompress(s.TypicalHitFileSystem)
	// err := json.Unmarshal(raw, &f)
	// if err != nil {
	// 	return nil, utils.Wrapf(err, `failed to unmarshal file system`)
	// }
	fs := filesys.NewVirtualFs()
	for filename, i := range f {
		raw, err := strconv.Unquote(i)
		if err != nil {
			continue
		}
		fs.AddFile(filename, raw)
	}
	return fs, nil
}
