package fstools

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/jar"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// CreateJarOperator creates AI tools for JAR operations
func CreateJarOperator() ([]*aitool.Tool, error) {
	var err error
	factory := aitool.NewFactory()

	// List JAR directory contents
	err = factory.RegisterTool("jar_list_directory",
		aitool.WithDescription("list files and directories in a JAR file"),
		aitool.WithStringParam("jar_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to JAR file"),
		),
		aitool.WithStringParam("dir_path",
			aitool.WithParam_Required(false),
			aitool.WithParam_Default("."),
			aitool.WithParam_Description("directory path inside JAR to list (default is root)"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			jarPath := params.GetString("jar_path")
			dirPath := params.GetString("dir_path")
			if dirPath == "" {
				dirPath = "."
			}

			jarParser, err := jar.NewJarParser(jarPath)
			if err != nil {
				return nil, utils.Errorf("failed to create JAR parser: %v", err)
			}

			contents, err := jarParser.GetDirectoryContents(dirPath)
			if err != nil {
				return nil, utils.Errorf("failed to list JAR directory '%s': %v", dirPath, err)
			}

			// Format the output as JSON array
			result, err := json.MarshalIndent(contents, "", "  ")
			if err != nil {
				return nil, utils.Errorf("failed to marshal directory contents: %v", err)
			}

			return string(result), nil
		}),
	)
	if err != nil {
		log.Errorf("register jar_list_directory tool: %v", err)
	}

	// Read a class file from JAR
	err = factory.RegisterTool("jar_read_class",
		aitool.WithDescription("read and decompile a class file from a JAR"),
		aitool.WithStringParam("jar_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to JAR file"),
		),
		aitool.WithStringParam("class_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to class file inside JAR (e.g., 'com/example/Main.class')"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			jarPath := params.GetString("jar_path")
			classPath := params.GetString("class_path")

			jarParser, err := jar.NewJarParser(jarPath)
			if err != nil {
				return nil, utils.Errorf("failed to create JAR parser: %v", err)
			}

			// Ensure classPath has .class extension
			if !strings.HasSuffix(classPath, ".class") {
				classPath += ".class"
			}

			// Use DecompileClass if it's a class file
			if strings.HasSuffix(classPath, ".class") {
				classData, err := jarParser.DecompileClass(classPath)
				if err != nil {
					return nil, utils.Errorf("failed to decompile class '%s': %v", classPath, err)
				}
				return string(classData), nil
			} else {
				// For non-class files, just read the content
				classData, err := jarParser.GetJarFS().ReadFile(classPath)
				if err != nil {
					return nil, utils.Errorf("failed to read file '%s': %v", classPath, err)
				}
				return string(classData), nil
			}
		}),
	)
	if err != nil {
		log.Errorf("register jar_read_class tool: %v", err)
	}

	// Read a file from JAR (non-class files)
	err = factory.RegisterTool("jar_read_file",
		aitool.WithDescription("read a file (non-class) from a JAR"),
		aitool.WithStringParam("jar_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to JAR file"),
		),
		aitool.WithStringParam("file_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to file inside JAR"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			jarPath := params.GetString("jar_path")
			filePath := params.GetString("file_path")

			jarParser, err := jar.NewJarParser(jarPath)
			if err != nil {
				return nil, utils.Errorf("failed to create JAR parser: %v", err)
			}

			fileData, err := jarParser.GetJarFS().ReadFile(filePath)
			if err != nil {
				return nil, utils.Errorf("failed to read file '%s': %v", filePath, err)
			}

			return string(fileData), nil
		}),
	)
	if err != nil {
		log.Errorf("register jar_read_file tool: %v", err)
	}

	// Get JAR manifest
	err = factory.RegisterTool("jar_get_manifest",
		aitool.WithDescription("read the manifest file from a JAR"),
		aitool.WithStringParam("jar_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to JAR file"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			jarPath := params.GetString("jar_path")

			jarParser, err := jar.NewJarParser(jarPath)
			if err != nil {
				return nil, utils.Errorf("failed to create JAR parser: %v", err)
			}

			manifest, err := jarParser.GetJarManifest()
			if err != nil {
				return nil, utils.Errorf("failed to read JAR manifest: %v", err)
			}

			// Format the output as JSON
			result, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return nil, utils.Errorf("failed to marshal manifest: %v", err)
			}

			return string(result), nil
		}),
	)
	if err != nil {
		log.Errorf("register jar_get_manifest tool: %v", err)
	}

	// Find classes in JAR
	err = factory.RegisterTool("jar_find_classes",
		aitool.WithDescription("find all Java classes in a JAR, including nested JARs"),
		aitool.WithStringParam("jar_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to JAR file"),
		),
		aitool.WithStringParam("include_nested_jars",
			aitool.WithParam_Required(false),
			aitool.WithParam_Default("true"),
			aitool.WithParam_Description("whether to include classes from nested JARs"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			jarPath := params.GetString("jar_path")
			includeNestedJars := params.GetBool("include_nested_jars")

			jarParser, err := jar.NewJarParser(jarPath)
			if err != nil {
				return nil, utils.Errorf("failed to create JAR parser: %v", err)
			}

			classes, err := jarParser.FindJavaClasses(includeNestedJars)
			if err != nil {
				return nil, utils.Errorf("failed to find Java classes: %v", err)
			}

			// Format the output as JSON array
			result, err := json.MarshalIndent(classes, "", "  ")
			if err != nil {
				return nil, utils.Errorf("failed to marshal class list: %v", err)
			}

			return string(result), nil
		}),
	)
	if err != nil {
		log.Errorf("register jar_find_classes tool: %v", err)
	}

	// Find a class by name
	err = factory.RegisterTool("jar_find_class_by_name",
		aitool.WithDescription("find a class by its name within a JAR file"),
		aitool.WithStringParam("jar_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("path to JAR file"),
		),
		aitool.WithStringParam("class_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("class name to find (with or without .class extension)"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			jarPath := params.GetString("jar_path")
			className := params.GetString("class_name")

			// Remove .class extension if present for search
			className = strings.TrimSuffix(className, ".class")

			jarParser, err := jar.NewJarParser(jarPath)
			if err != nil {
				return nil, utils.Errorf("failed to create JAR parser: %v", err)
			}

			classPath, err := jarParser.FindClassByName(className)
			if err != nil {
				return nil, utils.Errorf("failed to find class '%s': %v", className, err)
			}

			return classPath, nil
		}),
	)
	if err != nil {
		log.Errorf("register jar_find_class_by_name tool: %v", err)
	}
	return factory.Tools(), nil
}
