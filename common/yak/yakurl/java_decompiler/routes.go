package java_decompiler

import (
	"path/filepath"
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

		jarFS, err := a.getJarFS(jarPath)
		if err != nil {
			return nil, err
		}

		// Get directory to list
		dirPath, _ := getParam("dir")
		if dirPath == "" {
			dirPath = "."
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
		if dirPath == "" {
			dirPath = "."
		}

		jarFs, actualJarPath, currentDirPath, err := a.getNestedJarFs(jarPath, dirPath)
		if err != nil {
			return nil, err
		}
		return a.listJarDirectory(actualJarPath, dirPath, currentDirPath, jarFs, true)
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

		jarFs, actualJarPath, classPath, err := a.getNestedJarFs(jarPath, className)
		if err != nil {
			return nil, err
		}

		data, err := jarFs.ReadFile(classPath)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to read class: %s from jar: %s", classPath, jarPath)
		}

		// Create resource for the decompiled class
		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/class",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: actualJarPath},
				{Key: "class", Value: className},
			},
		}

		fileInfo, err := jarFs.Stat(classPath)
		if err != nil {
			return nil, err
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

		// Check if this is a nested jar path (like lib/aa.jar/main)
		jarFs, actualJarPath, classPath, err := a.getNestedJarFs(jarPath, className)
		if err != nil {
			return nil, err
		}
		// Get decompiled outer class content
		outerClassData, err := jarFs.ReadFile(classPath)
		if err != nil {
			content, err := jarFs.ZipFS.ReadFile(classPath)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to file: %s from jar: %s", className, jarPath)
			}
			resourceURL := &ypb.YakURL{
				Schema: "javaDec",
				Path:   "/class-aifix",
				Query: []*ypb.KVPair{
					{Key: "jar", Value: actualJarPath},
					{Key: "class", Value: className},
				},
			}

			fileInfo, err := jarFs.ZipFS.Stat(classPath)
			if err != nil {
				return nil, err
			}

			resource := a.createResourceFromFileInfo(resourceURL, fileInfo, className)
			resource.ResourceType = "class"
			resource.VerboseType = "ai-enhanced-java-class"
			resource.Extra = append(resource.Extra, &ypb.KVPair{
				Key:   "content",
				Value: codec.EncodeToHex(content),
			})
			return &ypb.RequestYakURLResponse{
				Resources: []*ypb.YakURLResource{resource},
				Total:     1,
				Page:      1,
				PageSize:  1,
			}, nil
		}

		// Get inner classes from the query parameters
		innerClassesParam, _ := getParam("innerClasses")
		innerClassesMap := make(map[string][]byte)

		if innerClassesParam != "" {
			// Use explicitly provided inner classes
			innerClassPaths := strings.Split(innerClassesParam, ",")
			for _, innerClassPath := range innerClassPaths {
				innerClassData, err := jarFs.ReadFile(innerClassPath)
				if err != nil {
					log.Warnf("Failed to read inner class: %s from jar: %s: %v", innerClassPath, jarPath, err)
					continue
				}

				_, innerClassName := filepath.Split(innerClassPath)
				innerClassesMap[innerClassName] = innerClassData
			}
		}

		// Call AI to combine the outer class with inner classes
		combinedCode, err := a.mockChatAI(outerClassData, innerClassesMap)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to process classes with AI")
		}

		// Create resource for the AI-enhanced decompiled class
		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/class-aifix",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: actualJarPath},
				{Key: "class", Value: className},
			},
		}

		fileInfo, err := jarFs.Stat(classPath)
		if err != nil {
			return nil, err
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
