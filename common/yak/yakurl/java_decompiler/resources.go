package java_decompiler

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// createResourceFromFileInfo creates a YakURLResource from fs.FileInfo
func (a *Action) createResourceFromFileInfo(url *ypb.YakURL, info fs.FileInfo, path string) *ypb.YakURLResource {
	_, fileName := filepath.Split(path)

	resource := &ypb.YakURLResource{
		Size:              info.Size(),
		SizeVerbose:       utils.ByteSize(uint64(info.Size())),
		ModifiedTimestamp: info.ModTime().Unix(),
		Path:              path,
		YakURLVerbose:     "",
		Url:               url,
		ResourceName:      fileName,
	}

	if info.IsDir() {
		resource.ResourceType = "dir"
		resource.VerboseType = "java-directory"
		resource.VerboseName = fileName
	} else {
		resource.ResourceType = "file"
		if strings.HasSuffix(path, ".class") {
			resource.VerboseType = "java-class"
		} else {
			resource.VerboseType = "java-file"
		}
		resource.VerboseName = fileName + " [" + resource.SizeVerbose + "]"
	}

	return resource
}
