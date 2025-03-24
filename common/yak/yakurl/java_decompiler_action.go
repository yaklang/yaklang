package yakurl

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type javaDecompilerAction struct {
	BaseAction
	jarFS map[string]*javaclassparser.FS
}

var _ Action = (*javaDecompilerAction)(nil)

func newJavaDecompilerAction() *javaDecompilerAction {
	action := &javaDecompilerAction{
		jarFS: make(map[string]*javaclassparser.FS),
	}

	// Handle JAR directory listing
	action.handle("GET", "/jar", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}

		jarFS, err := action.getJarFS(jarPath)
		if err != nil {
			return nil, err
		}

		// Get directory to list
		dirPath, _ := getParam("dir")
		if dirPath == "" {
			dirPath = "."
		}

		// List directory
		entries, err := jarFS.ReadDir(dirPath)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to read directory: %s in jar: %s", dirPath, jarPath)
		}

		resources := make([]*ypb.YakURLResource, 0, len(entries))
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				log.Errorf("failed to get info for %s: %v", entry.Name(), err)
				continue
			}

			entryPath := filepath.Join(dirPath, entry.Name())
			if entryPath == "./" {
				entryPath = entry.Name()
			}

			// Create a new URL for this resource
			resourceURL := &ypb.YakURL{
				Schema: "javaDec",
				Path:   "/jar",
			}

			if entry.IsDir() {
				// For directories, we'll update the dir parameter
				resourceURL.Query = append(resourceURL.Query, &ypb.KVPair{
					Key:   "jar",
					Value: jarPath,
				}, &ypb.KVPair{
					Key:   "dir",
					Value: entryPath,
				})
			} else {
				// For class files, we'll create a link to the decompiler/class endpoint
				if strings.HasSuffix(entry.Name(), ".class") {
					resourceURL.Path = "/class"
					resourceURL.Query = append(resourceURL.Query, &ypb.KVPair{
						Key:   "jar",
						Value: jarPath,
					}, &ypb.KVPair{
						Key:   "class",
						Value: entryPath,
					})
				} else {
					resourceURL.Query = append(resourceURL.Query, &ypb.KVPair{
						Key:   "jar",
						Value: jarPath,
					}, &ypb.KVPair{
						Key:   "dir",
						Value: filepath.Dir(entryPath),
					})
				}
			}

			resource := action.createResourceFromFileInfo(resourceURL, info, entryPath)

			// For directories, check if they have children
			if entry.IsDir() {
				subEntries, err := jarFS.ReadDir(entryPath)
				if err == nil {
					resource.HaveChildrenNodes = len(subEntries) > 0
				}
			}

			resources = append(resources, resource)
		}

		return &ypb.RequestYakURLResponse{
			Resources: resources,
			Total:     int64(len(resources)),
			Page:      1,
			PageSize:  int64(len(resources)),
		}, nil
	})

	// Handle class file decompilation
	action.handle("GET", "/class", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}

		className, err := getParam("class")
		if err != nil {
			return nil, utils.Error("class parameter is required")
		}

		jarFS, err := action.getJarFS(jarPath)
		if err != nil {
			return nil, err
		}

		// Get decompiled class content
		data, err := jarFS.ReadFile(className)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to read class: %s from jar: %s", className, jarPath)
		}

		// Create resource for the decompiled class
		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/class",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: jarPath},
				{Key: "class", Value: className},
			},
		}

		fileInfo, err := jarFS.Stat(className)
		if err != nil {
			return nil, err
		}

		resource := action.createResourceFromFileInfo(resourceURL, fileInfo, className)
		resource.ResourceType = "class"
		resource.VerboseType = "decompiled-java-class"
		resource.Extra = append(resource.Extra, &ypb.KVPair{
			Key:   "content",
			Value: codec.EncodeToHex(data),
		})

		return &ypb.RequestYakURLResponse{
			Resources: []*ypb.YakURLResource{resource},
			Total:     1,
			Page:      1,
			PageSize:  1,
		}, nil
	})

	return action
}

// getJarFS gets or creates a javaclassparser.FS for the given jar path
func (j *javaDecompilerAction) getJarFS(jarPath string) (*javaclassparser.FS, error) {
	if fs, ok := j.jarFS[jarPath]; ok {
		return fs, nil
	}

	fs, err := javaclassparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to open jar file: %s", jarPath)
	}
	j.jarFS[jarPath] = fs
	return fs, nil
}

// createResourceFromFileInfo creates a YakURLResource from fs.FileInfo
func (j *javaDecompilerAction) createResourceFromFileInfo(url *ypb.YakURL, info fs.FileInfo, path string) *ypb.YakURLResource {
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
