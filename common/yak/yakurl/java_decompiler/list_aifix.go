package java_decompiler

import (
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// listJarAifix lists a code source for the /jar-aifix route. It serves the same
// decompiled-source view as /jar, with plain display names and inner classes
// hidden below their outer class.
func (a *Action) listJarAifix(rootPath, dirPath string) (*ypb.RequestYakURLResponse, error) {
	if err := validateCodeSourceRoot(rootPath); err != nil {
		return nil, err
	}
	cs, err := a.resolveCodeSource(rootPath)
	if err != nil {
		return nil, err
	}

	dirPath = normalizeJarInternalPath(dirPath)
	entries, err := cs.listSourceEntries(dirPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to list directory: %s in code source: %s", dirPath, rootPath)
	}

	innerClassesByOuter, innerClassMap := innerClassIndex(entries)

	resources := make([]*ypb.YakURLResource, 0, len(entries))
	for _, entry := range entries {
		isDirLike := entry.Dir || javaclassparserIsArchiveLeaf(entry.Name)

		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/jar-aifix",
		}
		switch {
		case isDirLike:
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "dir", Value: entry.Path},
			}
		case entry.ClassPath != "":
			resourceURL.Path = "/class-aifix"
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "class", Value: entry.Path},
			}
		default:
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "dir", Value: path.Dir(entry.Path)},
			}
		}

		resource := a.createResourceFromFileInfo(resourceURL, entry.Info, entry.Path)
		resource.VerboseName = entry.Name

		if isDirLike {
			resource.ResourceType = "dir"
			resource.VerboseType = "java-directory"
			resource.HaveChildrenNodes = cs.hasChildren(entry.Path)
		}
		if entry.ClassPath != "" {
			resource.Extra = append(resource.Extra, &ypb.KVPair{
				Key:   "originalPath",
				Value: entry.ClassPath,
			})
			if innerClasses, ok := innerClassesByOuter[entry.Path]; ok {
				for _, innerClassPath := range innerClasses {
					resource.Extra = append(resource.Extra, &ypb.KVPair{
						Key:   "innerClass",
						Value: innerClassPath,
					})
				}
			}
			if outerClass, ok := innerClassMap[entry.Path]; ok {
				resource.Extra = append(resource.Extra,
					&ypb.KVPair{Key: "hide", Value: "true"},
					&ypb.KVPair{Key: "outerClass", Value: outerClass},
				)
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
}

func javaclassparserIsArchiveLeaf(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range javaArchiveExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

var javaArchiveExtensions = []string{".jar", ".war", ".ear", ".zip", ".par"}
