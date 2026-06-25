package java_decompiler

import (
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (a *Action) listJarAifix(rootPath, dirPath string) (*ypb.RequestYakURLResponse, error) {
	if err := validateCodeSourceRoot(rootPath); err != nil {
		return nil, err
	}
	cs, err := a.resolveCodeSource(rootPath)
	if err != nil {
		return nil, err
	}

	dirPath = normalizeJarInternalPath(dirPath)
	entries, err := cs.listDirectory(dirPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to list directory: %s in code source: %s", dirPath, rootPath)
	}

	innerClassesByOuter := make(map[string][]string)
	for _, entry := range entries {
		className := displayEntryName(entry)
		if !entry.IsDir() && strings.HasSuffix(className, ".class") {
			dollarIndex := strings.Index(className, "$")
			if dollarIndex > 0 {
				outerClassName := className[:dollarIndex] + ".class"
				entryPath := path.Join(dirPath, className)
				outerClassPath := path.Join(dirPath, outerClassName)
				innerClassesByOuter[outerClassPath] = append(innerClassesByOuter[outerClassPath], entryPath)
			}
		}
	}

	innerClassMap := make(map[string]string)
	for outerClass, innerClasses := range innerClassesByOuter {
		for _, innerClass := range innerClasses {
			innerClassMap[innerClass] = outerClass
		}
	}

	resources := make([]*ypb.YakURLResource, 0, len(entries))
	for _, entry := range entries {
		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/jar-aifix",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "dir", Value: dirPath},
			},
		}
		entryName := displayEntryName(entry)
		entryPath := path.Join(dirPath, entryName)
		fileInfo, err := entry.Info()
		if err != nil {
			return nil, err
		}

		if entry.IsDir() || javaclassparserIsArchiveLeaf(entryName) {
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "dir", Value: entryPath},
			}
		} else if strings.HasSuffix(entryName, ".class") {
			resourceURL.Path = "/class-aifix"
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "class", Value: entryPath},
			}
		} else {
			resourceURL.Query = []*ypb.KVPair{
				{Key: "jar", Value: rootPath},
				{Key: "dir", Value: path.Dir(entryPath)},
			}
		}

		resource := a.createResourceFromFileInfo(resourceURL, fileInfo, entryPath)

		if entry.IsDir() || javaclassparserIsArchiveLeaf(entryName) {
			resource.ResourceType = "dir"
			resource.VerboseType = "java-directory"
			resource.VerboseName = entryName
			resource.HaveChildrenNodes = true
		} else {
			resource.ResourceType = "file"
			resource.VerboseType = "java-file"
			resource.VerboseName = entryName

			if strings.HasSuffix(entryName, ".class") {
				if innerClasses, hasInnerClasses := innerClassesByOuter[entryPath]; hasInnerClasses {
					for _, innerClassPath := range innerClasses {
						resource.Extra = append(resource.Extra, &ypb.KVPair{
							Key:   "innerClass",
							Value: innerClassPath,
						})
					}
				}
				if outerClass, isInnerClass := innerClassMap[entryPath]; isInnerClass {
					resource.Extra = append(resource.Extra, &ypb.KVPair{
						Key:   "hide",
						Value: "true",
					})
					resource.Extra = append(resource.Extra, &ypb.KVPair{
						Key:   "outerClass",
						Value: outerClass,
					})
				}
			}
		}

		if entry.IsDir() || javaclassparserIsArchiveLeaf(entryName) {
			subEntries, err := cs.listDirectory(entryPath)
			if err == nil {
				resource.HaveChildrenNodes = len(subEntries) > 0
			} else {
				log.Warnf("failed to probe children for %s: %v", entryPath, err)
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
