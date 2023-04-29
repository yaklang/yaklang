package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type FileInfo struct {
	BuildIn os.FileInfo
	Path    string
	Name    string
	IsDir   bool
}

func ReadFilesRecursively(p string) ([]*FileInfo, error) {
	return readFilesRecursively(p, p, -1)
}

func ReadDir(p string) ([]*FileInfo, error) {
	var err error
	if !filepath.IsAbs(p) {
		p, err = filepath.Abs(p)
		if err != nil {
			return nil, err
		}
	}

	infos, err := ioutil.ReadDir(p)
	if err != nil {
		return nil, err
	}

	var ret []*FileInfo
	for _, i := range infos {
		ret = append(ret, &FileInfo{
			BuildIn: i,
			Path:    filepath.Join(p, i.Name()),
			Name:    i.Name(),
			IsDir:   i.IsDir(),
		})
	}
	return ret, nil
}

func ReadDirWithLimit(p string, limit int) ([]*FileInfo, error) {
	infos, err := ReadDir(p)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		return infos, nil
	}

	if len(infos) > limit {
		return infos[:limit], nil
	}

	return infos, nil
}

func ReadFilesRecursivelyWithLimit(p string, limit int) ([]*FileInfo, error) {
	return readFilesRecursively(p, p, limit)
}

func ReadDirsRecursively(p string) ([]*FileInfo, error) {
	files, err := ReadFilesRecursively(p)
	if err != nil {
		return nil, Errorf(err.Error())
	}

	var i []*FileInfo
	for _, info := range files {
		if info.IsDir {
			i = append(i, info)
		}
	}
	return i, nil
}

func readFilesRecursively(p string, baseDir string, limit int) ([]*FileInfo, error) {
	var err error
	if !filepath.IsAbs(p) {
		p, err = filepath.Abs(p)
		if err != nil {
			return nil, err
		}
	}

	e, err := PathExists(p)
	if err != nil {
		return nil, Errorf("judge path existed failed; %s", err)
	}

	if !e {
		return nil, Errorf("not existed path: %v", p)
	}

	infos, err := ioutil.ReadDir(p)
	if err != nil {
		return nil, err
	}

	var files []*FileInfo
	for _, info := range infos {
		if info.IsDir() {
			fs, err := readFilesRecursively(filepath.Join(p, info.Name()), baseDir, limit)
			if err != nil {
				continue
			}

			files = append(files, fs...)
		}

		path := filepath.Join(p, info.Name())
		name := info.Name()
		files = append(files, &FileInfo{
			BuildIn: info,
			IsDir:   info.IsDir(),
			Path:    path,
			Name:    name,
		})

		if limit <= 0 {
			continue
		}

		if len(files) > limit {
			break
		}
	}

	if limit <= 0 {
		return files, nil
	}

	if len(infos) > limit {
		return files[:limit], nil
	}

	return files, nil
}
