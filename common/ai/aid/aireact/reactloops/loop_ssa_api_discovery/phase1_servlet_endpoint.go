package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const servletEndpointHarvestWorkers = 4

// Servlet endpoint extraction regular expressions.
var (
	// web.xml patterns
	reServletMappingXML = regexp.MustCompile(`(?i)<servlet>[\s\S]*?<servlet-name>\s*([^<]+)\s*</servlet-name>[\s\S]*?<servlet-class>\s*([^<]+)\s*</servlet-class>[\s\S]*?</servlet>`)
	reServletURLPattern = regexp.MustCompile(`(?i)<servlet-mapping>[\s\S]*?<servlet-name>\s*([^<]+)\s*</servlet-name>[\s\S]*?<url-pattern>\s*([^<]+)\s*</url-pattern>[\s\S]*?</servlet-mapping>`)

	// Servlet annotation patterns
	reWebServlet       = regexp.MustCompile(`@WebServlet\s*\(\s*(?:value|urlPatterns)?\s*=\s*\{?\s*["']([^"']+)["']`)
	reWebServletMulti  = regexp.MustCompile(`@WebServlet\s*\(\s*\{[^}]*value\s*=\s*\{([^}]+)\}`)
	reWebFilter       = regexp.MustCompile(`@WebFilter\s*\(\s*(?:value|urlPatterns)?\s*=\s*\{?\s*["']([^"']+)["']`)
	reWebListener     = regexp.MustCompile(`@WebListener\s*\(`)
	reHTTPServlet      = regexp.MustCompile(`(?:extends|implements)\s+(?:Http)?Servlet\b`)

	// Servlet URL pattern variations
	reServletURLArray = regexp.MustCompile(`\{([^}]+)\}`)
)

// Servlet Endpoint Harvester.
type ServletEndpointHarvester struct {
	CodeRoot  string
	Endpoints []APIEndpoint
	// Servlet class to URL pattern mapping from XML
	ServletURLMap map[string][]string
}

// HarvestServletEndpoints extracts all Servlet endpoints from a Java project.
func HarvestServletEndpoints(codeRoot string) ([]APIEndpoint, error) {
	h := &ServletEndpointHarvester{
		CodeRoot:      codeRoot,
		Endpoints:     []APIEndpoint{},
		ServletURLMap: make(map[string][]string),
	}

	// Step 1: Extract from web.xml
	xmlEndpoints, _ := h.extractFromWebXML(codeRoot)
	h.Endpoints = append(h.Endpoints, xmlEndpoints...)

	// Step 2: Extract from @WebServlet annotations
	annoEndpoints, _ := h.extractFromAnnotations(codeRoot)
	h.Endpoints = append(h.Endpoints, annoEndpoints...)

	return deduplicateEndpoints(h.Endpoints), nil
}

// extractFromWebXML parses web.xml and extracts servlet mappings.
func (h *ServletEndpointHarvester) extractFromWebXML(codeRoot string) ([]APIEndpoint, error) {
	var endpoints []APIEndpoint

	// Find web.xml files
	patterns := []string{
		"**/web.xml",
		"**/WEB-INF/web.xml",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(codeRoot, pattern))
		for _, xmlPath := range matches {
			eps, _ := h.parseWebXML(xmlPath)
			endpoints = append(endpoints, eps...)
		}
	}

	return endpoints, nil
}

// parseWebXML parses a web.xml file and extracts servlet mappings.
func (h *ServletEndpointHarvester) parseWebXML(xmlPath string) ([]APIEndpoint, error) {
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(h.CodeRoot, xmlPath)
	relPath = filepath.ToSlash(relPath)

	content := string(data)
	var endpoints []APIEndpoint

	// Build servlet class -> servlet name mapping
	servletClasses := make(map[string]string)
	for _, m := range reServletMappingXML.FindAllStringSubmatch(content, -1) {
		if len(m) < 3 {
			continue
		}
		servletName := strings.TrimSpace(m[1])
		servletClass := strings.TrimSpace(m[2])
		servletClasses[servletClass] = servletName
		h.ServletURLMap[servletClass] = []string{}
	}

	// Build servlet name -> URL patterns mapping
	for _, m := range reServletURLPattern.FindAllStringSubmatch(content, -1) {
		if len(m) < 3 {
			continue
		}
		servletName := strings.TrimSpace(m[1])
		urlPattern := strings.TrimSpace(m[2])

		// Find the corresponding servlet class
		var servletClass string
		for sc, sn := range servletClasses {
			if sn == servletName {
				servletClass = sc
				break
			}
		}

		if servletClass == "" {
			continue
		}

		// Store URL pattern
		h.ServletURLMap[servletClass] = append(h.ServletURLMap[servletClass], urlPattern)

		// Create endpoint
		fqClass := servletClass
		id := generateEndpointID(fqClass, "service", "SERVLET", urlPattern)

		ep := APIEndpoint{
			ID:              id,
			Framework:       FrameworkServlet,
			ClassName:       fqClass,
			SimpleClassName: extractSimpleClassName(fqClass),
			MethodName:      "service", // Generic servlet service method
			PackageName:     extractPackageFromClass(fqClass),
			HTTPPath:        normalizeServletPath(urlPattern),
			HTTPMethods:     []string{"GET", "POST"}, // Servlets handle both
			AuthRequirements: []AuthRule{},
			AuthRequired:    false, // Servlets typically don't have method-level auth
			FilePath:        relPath,
			Confidence:      0.8,
			Provenance:      "xml_config",
		}

		endpoints = append(endpoints, ep)
	}

	return endpoints, nil
}

// extractFromAnnotations parses @WebServlet annotated classes.
func (h *ServletEndpointHarvester) extractFromAnnotations(codeRoot string) ([]APIEndpoint, error) {
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

		// Check if this is a Servlet class
		if !reHTTPServlet.MatchString(content) {
			return nil
		}

		// Extract @WebServlet annotations
		urlPatterns := extractWebServletURLs(content)
		if len(urlPatterns) == 0 {
			return nil
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

		fqClass := fqClassName(packageName, className)

		for _, urlPattern := range urlPatterns {
			id := generateEndpointID(fqClass, "service", "SERVLET", urlPattern)

			ep := APIEndpoint{
				ID:              id,
				Framework:       FrameworkServlet,
				ClassName:       fqClass,
				SimpleClassName: className,
				MethodName:      "service",
				PackageName:     packageName,
				HTTPPath:        normalizeServletPath(urlPattern),
				HTTPMethods:     []string{"GET", "POST"},
				AuthRequirements: []AuthRule{},
				AuthRequired:    false,
				FilePath:        relPath,
				Confidence:      0.75,
				Provenance:      "annotation",
			}

			endpoints = append(endpoints, ep)
		}

		return nil
	})

	return endpoints, err
}

// extractWebServletURLs extracts URL patterns from @WebServlet annotation.
func extractWebServletURLs(content string) []string {
	var urls []string

	// Single URL pattern: @WebServlet("/path")
	for _, m := range reWebServlet.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			url := strings.TrimSpace(m[1])
			if url != "" {
				urls = append(urls, url)
			}
		}
	}

	// Multiple URL patterns: @WebServlet(value = {"/path1", "/path2"})
	reURLArray := regexp.MustCompile(`@WebServlet\s*\(\s*\{[^}]*\}`)
	for _, m := range reURLArray.FindAllStringSubmatch(content, -1) {
		if len(m) > 0 {
			// Extract individual patterns
			reSingleURL := regexp.MustCompile(`["']([^"']+)["']`)
			for _, u := range reSingleURL.FindAllStringSubmatch(m[0], -1) {
				if len(u) > 1 {
					urls = append(urls, u[1])
				}
			}
		}
	}

	return urls
}

// normalizeServletPath normalizes servlet URL patterns.
func normalizeServletPath(pattern string) string {
	pattern = strings.TrimSpace(pattern)

	// Remove leading * (servlet wildcard)
	if strings.HasPrefix(pattern, "*") {
		pattern = pattern[1:]
	}

	// Ensure leading /
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}

	// Remove trailing /*
	if strings.HasSuffix(pattern, "/*") {
		pattern = strings.TrimSuffix(pattern, "/*")
		pattern += "/**" // Indicate wildcard
	}

	// .ext pattern
	if strings.HasPrefix(pattern, "*.") {
		return pattern // Keep as-is for extension mapping
	}

	// Normalize
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return "/"
	}

	return pattern
}

// ExportServletEndpointsToFile saves endpoints to a JSON file.
func ExportServletEndpointsToFile(endpoints []APIEndpoint, outputPath string) error {
	data, err := json.MarshalIndent(endpoints, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}
