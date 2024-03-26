package filesys

import (
	"github.com/gobwas/glob"
	"github.com/kr/fs"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Recursive recursively walk through the file system
// raw: the root path
// opts: options
// return: error
//
// Example:
// ```
// err := filesys.Recursive( //
//	"testdata",
//	filesys.dir(["cc", "dd"], filesys.onFileStat((name, info) => {})),
// )
// ```
func Recursive(raw string, opts ...Option) error {
	return recursive(make(map[string]struct{}), raw, opts...)
}

func recursive(visited map[string]struct{}, raw string, opts ...Option) (retErr error) {
	c := NewConfig()
	for _, opt := range opts {
		opt(c)
	}

	base := raw
	isDir := utils.IsDir(raw)

	if c.onStart != nil {
		if err := c.onStart(base, isDir); err != nil {
			return err
		}
	}

	if !isDir {
		// not a dir
		return nil
	}

	if _, isVisisted := visited[base]; isVisisted {
		log.Info("repeated visit: ", base)
		return nil
	} else {
		visited[base] = struct{}{}
	}

	if c.fileSystem == nil {
		return utils.Errorf("file system is nil")
	}
	walker := fs.WalkFS(raw, c.fileSystem)
	var nextChains []*exactChain

	var fileCount int64
	var dirCount int64
	var totalCount int64
	for walker.Step() {
		totalCount++
		if c.totalLimit > 0 && c.totalLimit < totalCount {
			return utils.Errorf("total count limit exceeded: %d", c.totalLimit)
		}
		if walker.Err() != nil {
			if !c.noStopWhenErr {
				return walker.Err()
			}
			continue
		}

		stat := walker.Stat()
		if stat == nil {
			continue
		}

		isDir := stat.IsDir()
		if c.onStat != nil {
			if err := c.onStat(isDir, walker.Path(), stat); err != nil && !c.noStopWhenErr {
				return err
			}
		}

		if isDir {
			dirCount++
			if c.onDirStat != nil {
				if err := c.onDirStat(walker.Path(), stat); err != nil && !c.noStopWhenErr {
					return err
				}
			}

			for _, dirDesc := range c.chains {
				if dirDesc.globIns == nil {
					globIns, err := glob.Compile(dirDesc.dirGlob, '/')
					if err != nil {
						return err
					}
					dirDesc.globIns = globIns
				}
				if dirDesc.globIns.Match(stat.Name()) {
					nextChains = append(nextChains, &exactChain{
						dirpath: walker.Path(),
						opts:    append(dirDesc.opts, WithFileSystem(c.fileSystem)),
					})
				}
			}
		} else {
			fileCount++
			if err := c.onFileStat(walker.Path(), stat); err != nil && !c.noStopWhenErr {
				return err
			}
		}

		if c.fileLimit > 0 && c.fileLimit < fileCount {
			return utils.Errorf("file count limit exceeded: %d", c.fileLimit)
		}

		if c.dirLimit > 0 && c.dirLimit < dirCount {
			return utils.Errorf("dir count limit exceeded: %d", c.dirLimit)
		}
	}

	for _, chain := range nextChains {
		if err := recursive(visited, chain.dirpath, chain.opts...); err != nil && !c.noStopWhenErr {
			retErr = utils.Errorf("error when recursive: %s", err)
			return
		}
	}

	return nil
}
