package java_decompiler

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/jar"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
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
		jarParser, err := jar.NewJarParser(jarPath)
		if err != nil {
			return nil, err
		}
		entries, err := jarParser.ListDirectory(dirPath)
		if err != nil {
			return nil, err
		}

		// Collect inner classes information
		innerClassesByOuter := make(map[string][]string)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".class") {
				className := entry.Name()
				// Check if this is an inner class (contains $ in name)
				dollarIndex := strings.Index(className, "$")
				if dollarIndex > 0 {
					// Get the outer class name
					outerClassName := className[:dollarIndex] + ".class"
					// Add this inner class to the list for its outer class
					entryPath := filepath.Join(dirPath, className)
					outerClassPath := filepath.Join(dirPath, outerClassName)
					innerClassesByOuter[outerClassPath] = append(innerClassesByOuter[outerClassPath], entryPath)
				}
			}
		}

		// Create a map to quickly check if a class is an inner class
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
					{Key: "jar", Value: jarPath},
					{Key: "dir", Value: dirPath},
				},
			}
			entryPath := filepath.Join(dirPath, entry.Name())
			fileInfo, err := entry.Info()
			if err != nil {
				return nil, err
			}

			if entryPath == "./" {
				entryPath = entry.Name()
			}

			// Update resource URL based on entry type
			if entry.IsDir() {
				// For directories, update the dir parameter
				resourceURL.Query = []*ypb.KVPair{
					{Key: "jar", Value: jarPath},
					{Key: "dir", Value: entryPath},
				}
			} else {
				// For class files, create a link to class-aifix endpoint
				if strings.HasSuffix(entry.Name(), ".class") {
					resourceURL.Path = "/class-aifix"
					resourceURL.Query = []*ypb.KVPair{
						{Key: "jar", Value: jarPath},
						{Key: "class", Value: entryPath},
					}
				} else {
					resourceURL.Query = []*ypb.KVPair{
						{Key: "jar", Value: jarPath},
						{Key: "dir", Value: filepath.Dir(entryPath)},
					}
				}
			}

			resource := a.createResourceFromFileInfo(resourceURL, fileInfo, entryPath)

			if entry.IsDir() {
				resource.ResourceType = "dir"
				resource.VerboseType = "java-directory"
				resource.VerboseName = entry.Name()
				resource.HaveChildrenNodes = true
			} else {
				resource.ResourceType = "file"
				resource.VerboseType = "java-file"
				resource.VerboseName = entry.Name()

				// Handle inner class relationships
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".class") {
					// If this is an outer class with inner classes, add them to Extra
					if innerClasses, hasInnerClasses := innerClassesByOuter[entryPath]; hasInnerClasses {
						for _, innerClassPath := range innerClasses {
							resource.Extra = append(resource.Extra, &ypb.KVPair{
								Key:   "innerClass",
								Value: innerClassPath,
							})
						}
					}

					// If this is an inner class, mark it as hidden and add reference to outer class
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

			resources = append(resources, resource)
		}

		return &ypb.RequestYakURLResponse{
			Resources: resources,
			Total:     int64(len(resources)),
			Page:      1,
			PageSize:  int64(len(resources)),
		}, nil
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
	a.Handle("GET", "/export", func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error) {
		jarPath, err := getParam("jar")
		if err != nil {
			return nil, utils.Error("jar parameter is required")
		}

		// Get the JAR filesystem
		jarFs, err := a.getJarFS(jarPath)
		if err != nil {
			return nil, err
		}

		// Create an in-memory buffer for the zip file
		var buf bytes.Buffer
		zipWriter := zip.NewWriter(&buf)

		// Walk through all files in the JAR and add them to the zip
		err = filesys.Recursive(".", filesys.WithFileSystem(jarFs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if info.IsDir() {
				// Create directory entries in the zip
				_, err := zipWriter.Create(s + "/")
				return err
			}

			// Create a new file entry in the zip
			var fileContent []byte
			var targetPath string

			if filepath.Ext(s) == ".class" {
				// For class files, decompile them and save as .java
				decompiled, err := jarFs.ReadFile(s)
				if err != nil {
					// If decompilation fails, use the original class file
					log.Warnf("Failed to decompile %s: %v", s, err)
					decompiled, err = jarFs.ZipFS.ReadFile(s)
					if err != nil {
						return utils.Wrapf(err, "failed to read class file: %s", s)
					}
					fileContent = decompiled
					targetPath = s
				} else {
					// Decompilation succeeded, save as .java
					fileContent = decompiled
					targetPath = strings.TrimSuffix(s, ".class") + ".java"
				}
			} else {
				// For non-class files, just copy them as is
				var err error
				fileContent, err = jarFs.ZipFS.ReadFile(s)
				if err != nil {
					return utils.Wrapf(err, "failed to read file: %s", s)
				}
				targetPath = s
			}

			// Create and write the file to the zip
			zipFile, err := zipWriter.Create(targetPath)
			if err != nil {
				return utils.Wrapf(err, "failed to create zip entry for: %s", targetPath)
			}

			_, err = zipFile.Write(fileContent)
			if err != nil {
				return utils.Wrapf(err, "failed to write content for: %s", targetPath)
			}

			return nil
		}))

		if err != nil {
			return nil, utils.Wrapf(err, "failed to process jar files")
		}

		// Close the zip writer to flush its contents
		err = zipWriter.Close()
		if err != nil {
			return nil, utils.Wrapf(err, "failed to close zip writer")
		}

		// Create a resource for the exported zip
		_, jarFileName := filepath.Split(jarPath)
		exportedFileName := strings.TrimSuffix(jarFileName, ".jar") + "-decompiled.zip"

		resourceURL := &ypb.YakURL{
			Schema: "javaDec",
			Path:   "/export",
			Query: []*ypb.KVPair{
				{Key: "jar", Value: jarPath},
			},
		}

		resource := &ypb.YakURLResource{
			ResourceName:      exportedFileName,
			VerboseName:       exportedFileName,
			ResourceType:      "file",
			VerboseType:       "decompiled-jar-zip",
			Size:              int64(buf.Len()),
			SizeVerbose:       utils.ByteSize(uint64(buf.Len())),
			ModifiedTimestamp: time.Now().Unix(),
			Path:              exportedFileName,
			Url:               resourceURL,
			Extra: []*ypb.KVPair{
				{
					Key:   "content",
					Value: codec.EncodeToHex(buf.Bytes()),
				},
			},
		}

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
		jarParser, err := jar.NewJarParser(jarPath)
		if err != nil {
			return nil, err
		}
		outerClassData, err := jarParser.DecompileClass(className)
		if err != nil {
			return nil, err
		}
		// Get inner classes from the query parameters
		innerClassesParam, _ := getParam("innerClasses")
		innerClassesMap := make(map[string][]byte)

		if innerClassesParam != "" {
			// Use explicitly provided inner classes
			innerClassPaths := strings.Split(innerClassesParam, ",")
			for _, innerClassPath := range innerClassPaths {
				innerClassData, err := jarParser.GetJarFS().ReadFile(innerClassPath)
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
				{Key: "jar", Value: jarPath},
				{Key: "class", Value: className},
			},
		}

		fileInfo, err := jarParser.GetJarFS().Stat(className)
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
