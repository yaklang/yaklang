package java_decompiler

import (
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// innerClassIndex groups inner classes below their outer class using the
// display paths, so /class and /class-aifix receive the same .java names the
// listing serves.
func innerClassIndex(entries []sourceEntry) (map[string][]string, map[string]string) {
	byOuter := make(map[string][]string)
	for _, entry := range entries {
		if entry.Dir || entry.ClassPath == "" {
			continue
		}
		dollarIndex := strings.Index(entry.Name, "$")
		if dollarIndex <= 0 {
			continue
		}
		outerPath := path.Join(path.Dir(entry.Path), entry.Name[:dollarIndex]+".java")
		byOuter[outerPath] = append(byOuter[outerPath], entry.Path)
	}
	byInner := make(map[string]string, len(byOuter))
	for outerPath, innerPaths := range byOuter {
		for _, innerPath := range innerPaths {
			byInner[innerPath] = outerPath
		}
	}
	return byOuter, byInner
}

// listCodeSourceDirectory lists one directory of a code source. Bytecode
// entries are presented as .java files and read through the decompiler.
func (a *Action) listCodeSourceDirectory(jarPath, dirPath string, currentDirPath string, cs *codeSource, hideInnerClasses bool) (*ypb.RequestYakURLResponse, error) {
	dirPath = normalizeJarInternalPath(dirPath)
	currentDirPath = normalizeJarInternalPath(currentDirPath)

	entries, err := cs.listSourceEntries(currentDirPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read directory: %s in code source: %s", currentDirPath, jarPath)
	}

	innerClassesByOuter, innerClassMap := innerClassIndex(entries)

	resources := make([]*ypb.YakURLResource, 0, len(entries))
	for _, entry := range entries {
		isDirLike := entry.Dir || javaclassparserIsArchiveLeaf(entry.Name)

		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/jar",
		}
		switch {
		case isDirLike:
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: jarPath},
				{Key: "dir", Value: entry.Path},
			}
		case entry.ClassPath != "":
			resourceURL.Path = "/class"
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: jarPath},
				{Key: "class", Value: entry.Path},
			}
		default:
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: jarPath},
				{Key: "dir", Value: path.Dir(entry.Path)},
			}
		}

		resource := a.createResourceFromFileInfo(resourceURL, entry.Info, entry.Path)
		if isDirLike {
			resource.HaveChildrenNodes = cs.hasChildren(entry.Path)
		}
		if entry.ClassPath != "" {
			resource.Extra = append(resource.Extra, &ypb.KVPair{
				Key:   "originalPath",
				Value: entry.ClassPath,
			})
		}

		if hideInnerClasses && entry.ClassPath != "" {
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

// hasChildren reports whether a directory or archive entry can be expanded.
func (cs *codeSource) hasChildren(entryPath string) bool {
	subEntries, err := cs.listDirectory(entryPath)
	if err != nil {
		log.Warnf("failed to probe children for %s: %v", entryPath, err)
		return false
	}
	return len(subEntries) > 0
}
