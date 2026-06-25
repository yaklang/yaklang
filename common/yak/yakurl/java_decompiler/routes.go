package java_decompiler

import (
	"io/fs"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// registerJarRoutes registers JAR directory listing routes
func (a *Action) registerJarRoutes() {
	// Handle JAR directory listing
	a.Handle("GET", "/jar", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}
		if err := validateCodeSourceRoot(jarPath); err != nil {
			return nil, err
		}

		dirPath, _ := getParam("dir")
		dirPath = normalizeJarInternalPath(dirPath)

		cs, err := a.resolveCodeSource(jarPath)
		if err != nil {
			return nil, err
		}
		if cs.isDirectory {
			return a.listCodeSourceDirectory(jarPath, dirPath, dirPath, cs, false)
		}

		jarFS, err := a.getJarFS(jarPath)
		if err != nil {
			return nil, err
		}

		return a.listJarDirectory(jarPath, dirPath, dirPath, jarFS, false)
	})

	// Handle AI-enhanced JAR directory listing
	a.Handle("GET", "/jar-aifix", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}
		dirPath, err := getParam("dir")
		if err != nil {
			return nil, utils.Errorf("dir parameter is required")
		}
		return a.listJarAifix(jarPath, dirPath)
	})
}

// registerClassRoutes registers class decompilation routes
func (a *Action) registerClassRoutes() {
	// Handle class file decompilation
	a.Handle("GET", "/class", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}

		className, err := getParam("class")
		if err != nil {
			return nil, utils.Error("class parameter is required")
		}
		className = normalizeJarInternalPath(className)

		if err := validateCodeSourceRoot(jarPath); err != nil {
			return nil, err
		}
		cs, err := a.resolveCodeSource(jarPath)
		if err != nil {
			return nil, err
		}

		var (
			data          []byte
			actualJarPath = jarPath
			classPath     = className
			fileInfo      fs.FileInfo
		)
		if cs.isDirectory {
			data, err = cs.readFile(className)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to read class: %s from code source: %s", className, jarPath)
			}
			fileInfo, err = cs.stat(className)
			if err != nil {
				return nil, err
			}
		} else {
			jarFs, nestedJarPath, nestedClassPath, err := a.getNestedJarFs(jarPath, className)
			if err != nil {
				return nil, err
			}
			data, err = jarFs.ReadFile(nestedClassPath)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to read class: %s from jar: %s", nestedClassPath, jarPath)
			}
			actualJarPath = nestedJarPath
			classPath = className
			fileInfo, err = jarFs.Stat(nestedClassPath)
			if err != nil {
				return nil, err
			}
		}

		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/class",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: actualJarPath},
				{Key: "class", Value: classPath},
			},
		}

		resource := a.createResourceFromFileInfo(resourceURL, fileInfo, className)
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
	a.Handle("GET", "/export", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}
		return a.exportDecompiledCodeSource(jarPath)
	})
	// Handle AI-enhanced class decompilation
	a.Handle("GET", "/class-aifix", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}

		className, err := getParam("class")
		if err != nil {
			return nil, utils.Error("class parameter is required")
		}
		className = normalizeJarInternalPath(className)

		if err := validateCodeSourceRoot(jarPath); err != nil {
			return nil, err
		}
		cs, err := a.resolveCodeSource(jarPath)
		if err != nil {
			return nil, err
		}

		var (
			outerClassData []byte
			actualJarPath  = jarPath
			classPath      = className
			fileInfo       fs.FileInfo
		)

		if cs.isDirectory {
			outerClassData, err = cs.readFile(className)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to read class: %s from code source: %s", className, jarPath)
			}
			fileInfo, err = cs.stat(className)
			if err != nil {
				return nil, err
			}
		} else {
			jarFs, nestedJarPath, nestedClassPath, err := a.getNestedJarFs(jarPath, className)
			if err != nil {
				return nil, err
			}
			outerClassData, err = jarFs.ReadFile(nestedClassPath)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to read class: %s from jar: %s", nestedClassPath, jarPath)
			}
			actualJarPath = nestedJarPath
			classPath = className
			fileInfo, err = jarFs.Stat(nestedClassPath)
			if err != nil {
				return nil, err
			}
		}

		innerClassesParam, _ := getParam("innerClasses")
		innerClassesMap := make(map[string][]byte)

		if innerClassesParam != "" {
			innerClassPaths := strings.Split(innerClassesParam, ",")
			for _, innerClassPath := range innerClassPaths {
				innerClassPath = normalizeJarInternalPath(innerClassPath)
				var innerClassData []byte
				if cs.isDirectory {
					innerClassData, err = cs.readFile(innerClassPath)
				} else {
					innerJarFs, _, innerClassPathParsed, err := a.getNestedJarFs(jarPath, innerClassPath)
					if err != nil {
						log.Warnf("Failed to parse inner class path: %s from jar: %s: %v", innerClassPath, jarPath, err)
						continue
					}
					innerClassData, err = innerJarFs.ReadFile(innerClassPathParsed)
					if err != nil {
						log.Warnf("Failed to read inner class: %s from jar: %s: %v", innerClassPath, jarPath, err)
						continue
					}
					innerClassPath = innerClassPathParsed
				}
				if err != nil {
					log.Warnf("Failed to read inner class: %s from code source: %s: %v", innerClassPath, jarPath, err)
					continue
				}
				_, innerClassName := path.Split(innerClassPath)
				innerClassesMap[innerClassName] = innerClassData
			}
		}

		// Call AI to combine the outer class with inner classes
		combinedCode, err := a.mockChatAI(outerClassData, innerClassesMap)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to process classes with AI")
		}

		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/class-aifix",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: actualJarPath},
				{Key: "class", Value: classPath},
			},
		}

		resource := a.createResourceFromFileInfo(resourceURL, fileInfo, className)
		resource.ResourceType = "class"
		resource.VerboseType = "ai-enhanced-java-class"
		resource.Extra = append(resource.Extra, &ypb.KVPair{
			Key:   "content",
			Value: codec.EncodeToHex(combinedCode),
		})

		// Add information about included inner classes
		if len(innerClassesMap) > 0 {
			// Collect keys manually instead of using maps.Keys
			innerClassNames := make([]string, 0, len(innerClassesMap))
			for innerClassName := range innerClassesMap {
				innerClassNames = append(innerClassNames, innerClassName)
			}
			innerClassesInfo := strings.Join(innerClassNames, ", ")
			resource.Extra = append(resource.Extra, &ypb.KVPair{
				Key:   "innerClassesIncluded",
				Value: innerClassesInfo,
			})
		}

		return &ypb.RequestYakURLResponse{
			Resources: []*ypb.YakURLResource{resource},
			Total:     1,
			Page:      1,
			PageSize:  1,
		}, nil
	})
}
