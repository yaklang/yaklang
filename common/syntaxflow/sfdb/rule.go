package sfdb

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io/fs"
	"strconv"
)

func LoadFileSystem(s *schema.SyntaxFlowRule, system filesys.FileSystem) error {
	f := make(map[string]string)
	filesys.Recursive(".", filesys.WithFileSystem(system), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		raw, err := system.ReadFile(s)
		if err != nil {
			return nil
		}
		f[s] = strconv.Quote(string(raw))
		return nil
	}))
	raw, err := json.Marshal(f)
	if err != nil {
		return utils.Wrapf(err, `failed to marshal file system`)
	}

	s.TypicalHitFileSystem, _ = utils.GzipCompress(raw)
	return nil
}

func BuildFileSystem(s *schema.SyntaxFlowRule) (filesys.FileSystem, error) {
	f := make(map[string]string)
	raw, _ := utils.GzipDeCompress(s.TypicalHitFileSystem)
	err := json.Unmarshal(raw, &f)
	if err != nil {
		return nil, utils.Wrapf(err, `failed to unmarshal file system`)
	}
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

func Valid(s *schema.SyntaxFlowRule) error {
	fs, err := BuildFileSystem(s)
	if err != nil {
		return err
	}
	prog, err := ssaapi.ParseProject(fs)
	if err != nil {
		return err
	}
	result, err := prog.SyntaxFlowWithError(s.Content)
	if err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return utils.Errorf(`runtime error: %v`, result.Errors)
	}
	s.Verified = true
	return nil
}
