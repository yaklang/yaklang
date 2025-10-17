package yakgit

import (
	"bytes"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"path/filepath"
	"strings"
)

type BlameLine struct {
	LineNumber int
	*git.Line
}

func (b *BlameLine) String() string {
	var buf bytes.Buffer
	buf.WriteString(b.Hash.String()[:8])
	buf.WriteString(" [")
	buf.WriteString(b.AuthorName)
	if b.Author != "" && !strings.Contains(b.Author, "noreply.github") {
		buf.WriteString("(")
		buf.WriteString(b.Author)
		buf.WriteString(")")
	}
	buf.WriteString(" " + b.Date.String())
	buf.WriteString(" ")
	buf.WriteString(fmt.Sprintf("%3d", b.LineNumber))
	buf.WriteString("] ")
	buf.WriteString(b.Text)
	return buf.String()
}

type BlameLines []*BlameLine

func (b BlameLines) String() string {
	var buf bytes.Buffer
	for _, line := range b {
		buf.WriteString(line.String())
		buf.WriteString("\n")
	}
	return buf.String()
}

func BlameWithCommit(repos string, fileName string, rev string) (BlameLines, error) {
	var absRepo = repos
	if !filepath.IsAbs(repos) {
		var err error
		absRepo, err = filepath.Abs(repos)
		if err != nil {
			return nil, err
		}
	}

	var abspath = fileName
	if !filepath.IsAbs(fileName) {
		targetFile := filepath.Join(absRepo, fileName)
		if ok, _ := utils.PathExists(targetFile); !ok {
			var err error
			abspath, err = filepath.Abs(fileName)
			if err != nil {
				return nil, utils.Errorf("%v is not in %v", fileName, repos)
			}
		} else {
			abspath = targetFile
		}
	}

	if ok, _ := utils.PathExists(abspath); !ok {
		return nil, utils.Errorf("file not exists: %v", abspath)
	}

	// 检查文件是否在仓库中
	relPath, err := filepath.Rel(absRepo, abspath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return nil, utils.Errorf("file %v is not in repository %v", fileName, repos)
	}

	r, err := GitOpenRepositoryWithCache(absRepo)
	if err != nil {
		return nil, err
	}

	hash, err := RevParse(repos, rev)
	if err != nil {
		return nil, utils.Errorf("cannot rev-parse %v: %v", rev, err)
	}

	cmt, err := r.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, err
	}

	blameTarget, err := filepath.Rel(absRepo, abspath)
	if err != nil {
		blameTarget = fileName
	}
	log.Infof("start to git-blame %v", blameTarget)
	result, err := git.Blame(cmt, blameTarget)
	if err != nil {
		return nil, utils.Errorf("blame[%v] failed: %v", abspath, err)
	}
	var lines = make([]*BlameLine, len(result.Lines))
	for i := range result.Lines {
		lines[i] = &BlameLine{
			LineNumber: i + 1,
			Line:       result.Lines[i],
		}
	}
	return lines, nil
}

func Blame(repos string, fileName string) (BlameLines, error) {
	return BlameWithCommit(repos, fileName, "HEAD")
}
