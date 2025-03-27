package java_decompiler

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// listJarDirectory lists the contents of a directory in a JAR file
// If hideInnerClasses is true, it will add hide=true and outerClass attributes to inner class resources
func (a *Action) listJarDirectory(jarPath, dirPath string, currentDirPath string, jarFS *javaclassparser.FS, hideInnerClasses bool) (*ypb.RequestYakURLResponse, error) {
	// List directory
	entries, err := jarFS.ReadDir(currentDirPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read directory: %s in jar: %s", currentDirPath, jarPath)
	}

	resources := make([]*ypb.YakURLResource, 0, len(entries))

	// If hideInnerClasses is true, first collect all inner classes
	innerClassesByOuter := make(map[string][]string)
	if hideInnerClasses {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".class") {
				className := entry.Name()
				// Check if this is an inner class (contains $ in name)
				dollarIndex := strings.Index(className, "$")
				if dollarIndex > 0 {
					// Get the outer class name
					outerClassName := className[:dollarIndex] + ".class"
					// Add this inner class to the list for its outer class
					entryPath := filepath.Join(currentDirPath, className)
					outerClassPath := filepath.Join(currentDirPath, outerClassName)
					innerClassesByOuter[outerClassPath] = append(innerClassesByOuter[outerClassPath], entryPath)
				}
			}
		}
	}
	innerClassMap := make(map[string]string)
	for outerClass, innerClasses := range innerClassesByOuter {
		for _, innerClass := range innerClasses {
			innerClassMap[innerClass] = outerClass
		}
	}

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

		resource := a.createResourceFromFileInfo(resourceURL, info, entryPath)

		// For directories, check if they have children
		if entry.IsDir() {
			actualEntryPath := filepath.Join(currentDirPath, entry.Name())
			subEntries, err := jarFS.ReadDir(actualEntryPath)
			if err == nil {
				resource.HaveChildrenNodes = len(subEntries) > 0
			}
		}

		// If this is an outer class with inner classes, add them to the Extra field
		if hideInnerClasses && !entry.IsDir() && strings.HasSuffix(entry.Name(), ".class") {
			if innerClasses, hasInnerClasses := innerClassesByOuter[entryPath]; hasInnerClasses {
				// Add all inner classes to this outer class
				for _, innerClassPath := range innerClasses {
					resource.Extra = append(resource.Extra, &ypb.KVPair{
						Key:   "innerClass",
						Value: innerClassPath,
					})
				}
			}

			// If this is an inner class, add the hide property
			if v, ok := innerClassMap[entryPath]; ok {
				resource.Extra = append(resource.Extra, &ypb.KVPair{
					Key:   "hide",
					Value: "true",
				})

				// Add the outer class path
				outerClassPath := v
				resource.Extra = append(resource.Extra, &ypb.KVPair{
					Key:   "outerClass",
					Value: outerClassPath,
				})
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
