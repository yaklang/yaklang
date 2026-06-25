package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const strutsEndpointHarvestWorkers = 4

// Struts 2 endpoint extraction regular expressions.
var (
	// XML configuration patterns
	reStrutsPackage = regexp.MustCompile(`<package[^>]*name\s*=\s*["']([^"']+)["'][^>]*namespace\s*=\s*["']([^"']+)["']`)
	reStrutsPackageSimple = regexp.MustCompile(`<package[^>]*name\s*=\s*["']([^"']+)["']`)
	reStrutsNamespace    = regexp.MustCompile(`namespace\s*=\s*["']([^"']+)["']`)
	reStrutsActionXML    = regexp.MustCompile(`<action\s+([^>]+)>`)
	reStrutsActionName   = regexp.MustCompile(`name\s*=\s*["']([^"']+)["']`)
	reStrutsActionClass  = regexp.MustCompile(`class\s*=\s*["']([^"']+)["']`)
	reStrutsActionMethod = regexp.MustCompile(`method\s*=\s*["']([^"']+)["']`)
	reStrutsResult       = regexp.MustCompile(`<result[^>]*name\s*=\s*["']([^"']+)["'][^>]*>([^<]+)</result>`)
	reStrutsResultDefault = regexp.MustCompile(`<result[^>]*>([^<]+)</result>`)

	// Struts 2 annotation patterns
	reStrutsActionAnnotation  = regexp.MustCompile(`@Action\s*\(\s*(?:value\s*=\s*)?["']([^"']+)["']`)
	reStrutsActionAnnotationFull = regexp.MustCompile(`@Action\s*\(\s*\{([^}]+)\}`)
	reStrutsNamespaceAnnotation = regexp.MustCompile(`@Namespace\s*\(\s*["']([^"']+)["']`)
	reStrutsResultsAnnotation  = regexp.MustCompile(`@Results\s*\(\s*\{([^}]+)\}`)
	reStrutsResultAnnotation  = regexp.MustCompile(`@Result\s*\(\s*(?:name\s*=\s*)?["']([^"']+)["'][^)]*location\s*=\s*["']([^"']+)["']`)
	reStrutsInterceptorRef    = regexp.MustCompile(`@InterceptorRef\s*\(\s*["']([^"']+)["']`)
	reStrutsAllowedMethods    = regexp.MustCompile(`allowed-methods\s*=\s*["']([^"']+)["']`)
)

// Struts 2 Endpoint Harvester.
type Struts2EndpointHarvester struct {
	CodeRoot    string
	Endpoints   []APIEndpoint
	PackageNSMap map[string]string // package name -> namespace
}

// HarvestStruts2Endpoints extracts all Struts 2 endpoints from a Java project.
func HarvestStruts2Endpoints(codeRoot string) ([]APIEndpoint, error) {
	h := &Struts2EndpointHarvester{
		CodeRoot:      codeRoot,
		Endpoints:     []APIEndpoint{},
		PackageNSMap:  make(map[string]string),
	}

	// Step 1: Extract from XML configuration files
	xmlEndpoints, _ := h.extractFromXML(codeRoot)
	h.Endpoints = append(h.Endpoints, xmlEndpoints...)

	// Step 2: Extract from annotations
	annoEndpoints, _ := h.extractFromAnnotations(codeRoot)
	h.Endpoints = append(h.Endpoints, annoEndpoints...)

	// Deduplicate
	return deduplicateEndpoints(h.Endpoints), nil
}

// extractFromXML parses struts.xml and struts-*.xml files.
func (h *Struts2EndpointHarvester) extractFromXML(codeRoot string) ([]APIEndpoint, error) {
	var endpoints []APIEndpoint

	// Find all Struts configuration files
	patterns := []string{
		"**/struts.xml",
		"**/struts-*.xml",
		"**/struts/**/*.xml",
		"**/WEB-INF/struts*.xml",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(codeRoot, pattern))
		for _, xmlPath := range matches {
			eps, _ := h.parseStrutsXML(xmlPath)
			endpoints = append(endpoints, eps...)
		}
	}

	return endpoints, nil
}

// parseStrutsXML parses a single Struts XML configuration file.
func (h *Struts2EndpointHarvester) parseStrutsXML(xmlPath string) ([]APIEndpoint, error) {
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(h.CodeRoot, xmlPath)
	relPath = filepath.ToSlash(relPath)

	var endpoints []APIEndpoint
	content := string(data)

	// Find all packages
	packageMatches := reStrutsPackage.FindAllStringSubmatch(content, -1)
	for _, pkgMatch := range packageMatches {
		if len(pkgMatch) < 3 {
			continue
		}
		pkgName := pkgMatch[1]
		namespace := pkgMatch[2]

		// Find all actions in this package
		packageContent := extractXMLBlock(content, "package", pkgName)
		if packageContent == "" {
			packageContent = content
		}

		actionMatches := reStrutsActionXML.FindAllStringSubmatch(packageContent, -1)
		for _, actionMatch := range actionMatches {
			if len(actionMatch) < 2 {
				continue
			}

			actionAttrs := actionMatch[1]

			// Extract action attributes
			var name, class, method string
			var allowedMethods []string

			if m := reStrutsActionName.FindStringSubmatch(actionAttrs); len(m) > 1 {
				name = m[1]
			}
			if m := reStrutsActionClass.FindStringSubmatch(actionAttrs); len(m) > 1 {
				class = m[1]
			}
			if m := reStrutsActionMethod.FindStringSubmatch(actionAttrs); len(m) > 1 {
				method = m[1]
			}

			// Extract allowed methods if present
			if m := reStrutsAllowedMethods.FindStringSubmatch(actionAttrs); len(m) > 1 {
				allowedMethods = strings.Split(m[1], ",")
				for i := range allowedMethods {
					allowedMethods[i] = strings.TrimSpace(allowedMethods[i])
				}
			}

			// Default method
			if method == "" {
				method = "execute"
			}

			// Generate URL path
			urlPath := buildStrutsPath(namespace, name)

			// Extract results as potential HTTP methods (though Struts is POST by default)
			actionBlock := extractXMLBlock(packageContent, "action", name)
			var resultNames []string
			for _, m := range reStrutsResult.FindAllStringSubmatch(actionBlock, -1) {
				if len(m) > 1 {
					resultNames = append(resultNames, m[1])
				}
			}

			// Generate unique ID
			id := generateEndpointID(class, method, "ACTION", urlPath)

			ep := APIEndpoint{
				ID:              id,
				Framework:       FrameworkStruts2,
				ClassName:       class,
				SimpleClassName: extractSimpleClassName(class),
				MethodName:      method,
				PackageName:     extractPackageFromClass(class),
				HTTPPath:        urlPath,
				HTTPMethods:     []string{"POST"}, // Struts default
				AuthRequirements: []AuthRule{},  // XML-based auth needs separate analysis
				AuthRequired:    true,             // Most Struts apps have auth
				FilePath:        relPath,
				Confidence:      0.85, // XML config is reliable
				Provenance:      "xml_config",
			}

			// Add allowed methods if specified
			if len(allowedMethods) > 0 {
				ep.HTTPMethods = allowedMethods
			}

			endpoints = append(endpoints, ep)
		}
	}

	return endpoints, nil
}

// extractFromAnnotations parses Struts 2 Convention plugin annotations.
func (h *Struts2EndpointHarvester) extractFromAnnotations(codeRoot string) ([]APIEndpoint, error) {
	var endpoints []APIEndpoint

	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".java") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(h.CodeRoot, path)
		relPath = filepath.ToSlash(relPath)
		content := string(data)

		// Check if this is a Struts action class
		isAction := false
		if strings.Contains(content, "@Action") ||
			strings.Contains(content, "extends ActionSupport") ||
			strings.Contains(content, "implements Action") {
			isAction = true
		}

		if !isAction {
			return nil
		}

		// Extract class-level namespace
		classNamespace := ""
		if m := reStrutsNamespaceAnnotation.FindStringSubmatch(content); len(m) > 1 {
			classNamespace = m[1]
		}

		// Extract class name and package
		className := ""
		packageName := ""
		if m := reClassDecl.FindStringSubmatch(content); len(m) > 1 {
			className = m[1]
		}
		if m := reJavaPackage.FindSubmatch(data); len(m) > 1 {
			packageName = string(m[1])
		}

		// Find all @Action annotations
		actionMatches := reStrutsActionAnnotation.FindAllStringSubmatch(content, -1)
		for _, match := range actionMatches {
			if len(match) < 2 {
				continue
			}
			actionPath := match[1]

			// Find the method this annotation is attached to
			methodName := extractStrutsActionMethod(content, actionPath)

			// Generate URL path
			urlPath := buildStrutsPath(classNamespace, actionPath)

			fqClass := fqClassName(packageName, className)
			id := generateEndpointID(fqClass, methodName, "ACTION", urlPath)

			ep := APIEndpoint{
				ID:              id,
				Framework:       FrameworkStruts2,
				ClassName:       fqClass,
				SimpleClassName: className,
				MethodName:      methodName,
				PackageName:     packageName,
				HTTPPath:        urlPath,
				HTTPMethods:     []string{"POST"},
				AuthRequirements: []AuthRule{},
				AuthRequired:    true,
				FilePath:        relPath,
				Confidence:      0.8, // Annotation-based is reliable
				Provenance:      "annotation",
			}

			endpoints = append(endpoints, ep)
		}

		return nil
	})

	return endpoints, err
}

// extractStrutsActionMethod finds the method name for an @Action annotation.
func extractStrutsActionMethod(content, actionPath string) string {
	// Split content around the @Action annotation
	idx := strings.Index(content, "@Action")
	if idx < 0 {
		return "execute"
	}

	after := content[idx:]
	// Look for "public String " pattern after @Action
	reMethod := regexp.MustCompile(`public\s+String\s+(\w+)\s*\(`)
	if m := reMethod.FindStringSubmatch(after); len(m) > 1 {
		return m[1]
	}

	return "execute"
}

// Helper functions.

func buildStrutsPath(namespace, actionName string) string {
	namespace = strings.TrimSpace(namespace)
	actionName = strings.TrimSpace(actionName)

	var result string
	if namespace != "" && namespace != "/" {
		result = namespace
		if !strings.HasSuffix(result, "/") && !strings.HasPrefix(actionName, "/") {
			result += "/"
		}
	}
	if actionName != "" {
		result += actionName
	}

	result = strings.Trim(result, "/")
	if result == "" {
		return "/"
	}
	return "/" + result
}

func extractXMLBlock(content, tag, name string) string {
	// Simple block extraction - not robust for nested tags
	start := strings.Index(content, "<"+tag)
	if start < 0 {
		return ""
	}
	end := strings.Index(content[start:], "</"+tag+">")
	if end < 0 {
		return content[start : start+2000] // Truncate
	}
	return content[start : start+end+len(tag)+3]
}

func extractSimpleClassName(fqClass string) string {
	parts := strings.Split(fqClass, ".")
	return parts[len(parts)-1]
}

func extractPackageFromClass(fqClass string) string {
	parts := strings.Split(fqClass, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], ".")
}

// ExportStrutsEndpointsToFile saves endpoints to a JSON file.
func ExportStrutsEndpointsToFile(endpoints []APIEndpoint, outputPath string) error {
	data, err := json.MarshalIndent(endpoints, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}
