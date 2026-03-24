package aitool

import (
	"io"
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const MaxOutputFileBytes int64 = 40 * 1024

type OutputFileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Content string `json:"content,omitempty"`
}

func (o *OutputFileInfo) LineNumberedContent() string {
	if o.Content == "" {
		return ""
	}
	return utils.PrefixLinesWithLineNumbers(o.Content)
}

func (o *OutputFileInfo) IsSafeSize() bool {
	return o.Size <= MaxOutputFileBytes
}

func ReadOutputFileFromPath(path string) (*OutputFileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, utils.Errorf("stat output file %s failed: %v", path, err)
	}

	if fi.IsDir() {
		return nil, utils.Errorf("output file %s is a directory", path)
	}

	info := &OutputFileInfo{
		Path: path,
		Size: fi.Size(),
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, utils.Errorf("open output file %s failed: %v", path, err)
	}
	defer f.Close()

	if fi.Size() > MaxOutputFileBytes {
		buf := make([]byte, MaxOutputFileBytes)
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return nil, utils.Errorf("read output file %s failed: %v", path, err)
		}
		info.Content = string(buf[:n])
		log.Infof("output file %s truncated from %d to %d bytes", path, fi.Size(), n)
	} else {
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, utils.Errorf("read output file %s failed: %v", path, err)
		}
		info.Content = string(data)
	}

	return info, nil
}
