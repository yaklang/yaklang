package loop_ssa_api_discovery

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const jaxrsEndpointHarvestWorkers = 8

// JAX-RS endpoint extraction regular expressions.
var (
	// JAX-RS class-level annotations
	reJAXRSRootResource  = regexp.MustCompile(`@Path\s*\(\s*["']([^"']+)["']`)
	reJAXRSApplication  = regexp.MustCompile(`@ApplicationPath\s*\(\s*["']([^"']+)["']`)

	// JAX-RS method annotations
	reJAXRSGET    = regexp.MustCompile(`@GET\s*(?:\(|;|\{|$)`)
	reJAXRSPOST   = regexp.MustCompile(`@POST\s*(?:\(|;|\{|$)`)
	reJAXRSPUT    = regexp.MustCompile(`@PUT\s*(?:\(|;|\{|$)`)
	reJAXRSDELETE  = regexp.MustCompile(`@DELETE\s*(?:\(|;|\{|$)`)
	reJAXRSPATCH  = regexp.MustCompile(`@PATCH\s*(?:\(|;|\{|$)`)
	reJAXRSHEAD   = regexp.MustCompile(`@HEAD\s*(?:\(|;|\{|$)`)
	reJAXRSOPTIONS = regexp.MustCompile(`@OPTIONS\s*(?:\(|;|\{|$)`)
	reJAXRSMethodAnnot = regexp.MustCompile(`@(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s*(?:\(|;|\{|$)`)

	// JAX-RS path and parameter annotations
	reJAXRSPath    = regexp.MustCompile(`@Path\s*\(\s*["']([^"']+)["']`)
	reJAXRSPathParam  = regexp.MustCompile(`@PathParam\s*\(\s*["']?(\w+)["']?\s*\)`)
	reJAXRSQueryParam = regexp.MustCompile(`@QueryParam\s*\(\s*["']?(\w+)["']?\s*\)`)
	reJAXRSHeaderParam = regexp.MustCompile(`@HeaderParam\s*\(\s*["']?(\w+)["']?\s*\)`)
	reJAXRSMatrixParam = regexp.MustCompile(`@MatrixParam\s*\(\s*["']?(\w+)["']?\s*\)`)
	reJAXRSCookieParam = regexp.MustCompile(`@CookieParam\s*\(\s*["']?(\w+)["']?\s*\)`)
	reJAXRSFormParam   = regexp.MustCompile(`@FormParam\s*\(\s*["']?(\w+)["']?\s*\)`)

	// Request/Response body
	reConsumes = regexp.MustCompile(`@Consumes\s*\(\s*["']([^"']+)["']`)
	reProduces = regexp.MustCompile(`@Produces\s*\(\s*["']([^"']+)["']`)

	// Security annotations (JSR-250)
	reJAXRSPermitAll  = regexp.MustCompile(`@PermitAll\s*(?:\(|;|\{|\s|$)`)
	reJAXRSDenyAll    = regexp.MustCompile(`@DenyAll\s*(?:\(|;|\{|\s|$)`)
	reJAXRSRolesAllowed = regexp.MustCompile(`@RolesAllowed\s*\(\s*(?:\{?\s*)?["']?([^"'\)]+)["']?\s*(?:\})?\)`)
	reJAXRSRolesAllowedArray = regexp.MustCompile(`@RolesAllowed\s*\(\s*\{[\s\S]*?\}`)

	// Context injection
	reContext = regexp.MustCompile(`@Context\s+`)
)

// JAX-RS Endpoint Harvester.
type JAXRSEndpointHarvester struct {
	CodeRoot      string
	ApplicationPath string
	Endpoints     []APIEndpoint
	mu            sync.Mutex
}

// HarvestJAXRSEndpoints extracts all JAX-RS endpoints from a Java project.
func HarvestJAXRSEndpoints(codeRoot string) ([]APIEndpoint, error) {
	h := &JAXRSEndpointHarvester{
		CodeRoot:  codeRoot,
		Endpoints: []APIEndpoint{},
	}

	var javaFiles []string
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
		if strings.HasSuffix(strings.ToLower(path), ".java") {
			javaFiles = append(javaFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(javaFiles) == 0 {
		return nil, nil
	}

	// First pass: find ApplicationPath
	h.findApplicationPath(codeRoot)

	// Concurrent processing
	jobs := make(chan string, len(javaFiles))
	var wg sync.WaitGroup
	n := jaxrsEndpointHarvestWorkers
	if n > len(javaFiles) {
		n = len(javaFiles)
	}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for abs := range jobs {
				eps := h.extractFromFile(abs)
				if len(eps) > 0 {
					h.mu.Lock()
					h.Endpoints = append(h.Endpoints, eps...)
					h.mu.Unlock()
				}
			}
		}()
	}

	for _, f := range javaFiles {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	return deduplicateEndpoints(h.Endpoints), nil
}

// findApplicationPath scans for @ApplicationPath annotation.
func (h *JAXRSEndpointHarvester) findApplicationPath(codeRoot string) {
	javaFiles, _ := filepath.Glob(filepath.Join(codeRoot, "**/*.java"))
	for _, f := range javaFiles {
		if data, err := os.ReadFile(f); err == nil {
			if m := reJAXRSApplication.FindSubmatch(data); len(m) > 1 {
				h.ApplicationPath = string(m[1])
				return
			}
		}
	}
}

// extractFromFile extracts JAX-RS endpoints from a single Java file.
func (h *JAXRSEndpointHarvester) extractFromFile(absPath string) []APIEndpoint {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(h.CodeRoot, absPath)
	relPath = filepath.ToSlash(relPath)

	var endpoints []APIEndpoint
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var classBasePath string
	var className string
	var packageName string
	var pendingAnnotations []string
	var pendingLineNum int
	var currentLine int
	var inResourceClass bool

	// Track class-level @Path
	classPath := ""
	if m := reJAXRSRootResource.FindSubmatch(data); len(m) > 1 {
		classPath = string(m[1])
		classBasePath = classPath
		inResourceClass = true
	}

	if m := reJavaPackage.FindSubmatch(data); len(m) > 1 {
		packageName = string(m[1])
	}

	if m := reClassDecl.FindSubmatch(data); len(m) > 1 {
		className = string(m[1])
	}

	if !inResourceClass {
		return nil // Not a JAX-RS resource class
	}

	extractMethodEndpoints := func(methodLine string, methodLineNum int) {
		if className == "" {
			return
		}

		methodName := extractJavaMethodName(methodLine)
		if strings.TrimSpace(methodName) == "" {
			return
		}

		fqClass := fqClassName(packageName, className)

		// Extract HTTP methods
		var httpMethods []string
		var methodPaths []string
		var authRules []AuthRule
		var consumes string
		var _produces string // reserved for future use

		for _, anno := range pendingAnnotations {
			anno = strings.TrimSpace(anno)

			// Extract HTTP method
			if reJAXRSGET.MatchString(anno) {
				httpMethods = append(httpMethods, "GET")
			} else if reJAXRSPOST.MatchString(anno) {
				httpMethods = append(httpMethods, "POST")
			} else if reJAXRSPUT.MatchString(anno) {
				httpMethods = append(httpMethods, "PUT")
			} else if reJAXRSDELETE.MatchString(anno) {
				httpMethods = append(httpMethods, "DELETE")
			} else if reJAXRSPATCH.MatchString(anno) {
				httpMethods = append(httpMethods, "PATCH")
			} else if reJAXRSHEAD.MatchString(anno) {
				httpMethods = append(httpMethods, "HEAD")
			} else if reJAXRSOPTIONS.MatchString(anno) {
				httpMethods = append(httpMethods, "OPTIONS")
			}

			// Extract method-level @Path
			if m := reJAXRSPath.FindStringSubmatch(anno); len(m) > 1 {
				methodPaths = append(methodPaths, m[1])
			}

			// Extract @Produces/@Consumes
			_ = _produces // reserved for future use
			if m := reConsumes.FindStringSubmatch(anno); len(m) > 1 {
				consumes = m[1]
			}

			// Extract auth rules
			authRules = append(authRules, extractJAXRSAuthRules(anno)...)
		}

		// Only process if we found HTTP method annotations
		if len(httpMethods) == 0 {
			pendingAnnotations = pendingAnnotations[:0]
			return
		}

		// Default path
		if len(methodPaths) == 0 {
			methodPaths = []string{""}
		}

		// Generate endpoint for each (method, path) combination
		for _, httpMethod := range httpMethods {
			for _, methodPath := range methodPaths {
				fullPath := h.composePath(classBasePath, methodPath)

				// Extract path variables from method signature
				pathVars := extractJAXRSPathVars(methodLine)

				// Generate unique ID
				id := generateEndpointID(fqClass, methodName, httpMethod, fullPath)

				// Auth required?
				authRequired := !isJAXRSPermitAll(pendingAnnotations) && !isJAXRSDenyAll(pendingAnnotations)

				ep := APIEndpoint{
					ID:              id,
					Framework:       FrameworkJAXRS,
					ClassName:       fqClass,
					SimpleClassName: className,
					MethodName:      methodName,
					PackageName:     packageName,
					HTTPPath:        fullPath,
					HTTPMethods:     []string{httpMethod},
					PathVariables:   pathVars,
					QueryParams:     extractJAXRSQueryParams(methodLine),
					RequestBody:     extractJAXRSRequestBody(methodLine, consumes),
					AuthRequirements: authRules,
					AuthRequired:   authRequired,
					FilePath:        relPath,
					LineNumber:      methodLineNum,
					Confidence:      computeJAXRSConfidence(authRules, pathVars),
					Provenance:      "annotation",
					RawAnnotations:  pendingAnnotations,
				}

				endpoints = append(endpoints, ep)
			}
		}

		pendingAnnotations = pendingAnnotations[:0]
	}

	for sc.Scan() {
		currentLine++
		line := sc.Text()
		t := strings.TrimSpace(line)

		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}

		// Annotation line
		if strings.HasPrefix(t, "@") {
			pendingAnnotations = append(pendingAnnotations, t)
			pendingLineNum = currentLine
			continue
		}

		// Method declaration
		if isPublicMethod(t) && strings.Contains(t, "(") {
			extractMethodEndpoints(t, pendingLineNum)
			continue
		}

		// Reset annotations
		if !strings.HasPrefix(t, "@") && len(pendingAnnotations) > 0 {
			if !strings.Contains(t, "class ") && !strings.Contains(t, "interface ") && !strings.Contains(t, "enum ") {
				pendingAnnotations = pendingAnnotations[:0]
			}
		}
	}

	return endpoints
}

// composePath combines class and method paths according to JAX-RS spec.
func (h *JAXRSEndpointHarvester) composePath(classPath, methodPath string) string {
	classPath = strings.TrimSpace(classPath)
	methodPath = strings.TrimSpace(methodPath)

	// Apply application path prefix
	appPrefix := h.ApplicationPath
	if appPrefix == "" {
		appPrefix = ""
	}

	// Combine paths
	var result string
	if appPrefix != "" {
		result = appPrefix
	}
	if classPath != "" {
		if !strings.HasPrefix(classPath, "/") && result != "" {
			classPath = "/" + classPath
		}
		result += classPath
	}
	if methodPath != "" {
		if !strings.HasPrefix(methodPath, "/") && result != "" {
			methodPath = "/" + methodPath
		}
		result += methodPath
	}

	// Normalize
	if result == "" {
		return "/"
	}
	result = normURLPath(result)
	if result == "/" {
		return result
	}
	return strings.TrimSuffix(result, "/")
}

// Helper functions.

func extractJAXRSAuthRules(anno string) []AuthRule {
	var rules []AuthRule

	if reJAXRSPermitAll.MatchString(anno) {
		rules = append(rules, AuthRule{
			Type: "permit_all",
		})
	}

	if reJAXRSDenyAll.MatchString(anno) {
		rules = append(rules, AuthRule{
			Type: "deny_all",
		})
	}

	if m := reJAXRSRolesAllowed.FindStringSubmatch(anno); len(m) > 1 {
		roles := splitJAXRSRoles(m[1])
		rules = append(rules, AuthRule{
			Type:  "pre_authz",
			Roles: roles,
		})
	}

	return rules
}

func splitJAXRSRoles(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "{} \t\"'-")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var roles []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"'- ")
		if p != "" {
			roles = append(roles, p)
		}
	}
	return roles
}

func isJAXRSPermitAll(annos []string) bool {
	for _, a := range annos {
		if reJAXRSPermitAll.MatchString(a) {
			return true
		}
	}
	return false
}

func isJAXRSDenyAll(annos []string) bool {
	for _, a := range annos {
		if reJAXRSDenyAll.MatchString(a) {
			return true
		}
	}
	return false
}

func extractJAXRSPathVars(methodLine string) []PathVariable {
	var vars []PathVariable
	for _, m := range reJAXRSPathParam.FindAllStringSubmatch(methodLine, -1) {
		if len(m) > 1 {
			vars = append(vars, PathVariable{
				Name: m[1],
				Type: "String",
			})
		}
	}
	return vars
}

func extractJAXRSQueryParams(methodLine string) []QueryParam {
	var params []QueryParam
	for _, m := range reJAXRSQueryParam.FindAllStringSubmatch(methodLine, -1) {
		if len(m) > 1 {
			params = append(params, QueryParam{
				Name:     m[1],
				Required: false,
			})
		}
	}
	return params
}

func extractJAXRSRequestBody(methodLine, consumes string) *RequestBody {
	// JAX-RS uses entity parameter (no annotation) for request body
	// Look for parameters without JAX-RS param annotations
	hasBody := false
	for _, m := range reJAXRSPathParam.FindAllStringSubmatch(methodLine, -1) {
		_ = m
		hasBody = true
	}
	for _, m := range reJAXRSQueryParam.FindAllStringSubmatch(methodLine, -1) {
		_ = m
		hasBody = true
	}
	for _, m := range reJAXRSHeaderParam.FindAllStringSubmatch(methodLine, -1) {
		_ = m
		hasBody = true
	}

	if !hasBody && strings.Contains(methodLine, "String") ||
		strings.Contains(methodLine, "InputStream") ||
		strings.Contains(methodLine, "byte[]") {
		// Likely has a body parameter
		contentType := consumes
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return &RequestBody{
			ContentType: contentType,
		}
	}

	return nil
}

func computeJAXRSConfidence(authRules []AuthRule, pathVars []PathVariable) float64 {
	confidence := 0.7

	if len(authRules) > 0 {
		confidence += 0.15
	}

	if len(pathVars) > 0 {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// ExportJAXRSEndpointsToFile saves endpoints to a JSON file.
func ExportJAXRSEndpointsToFile(endpoints []APIEndpoint, outputPath string) error {
	data, err := json.MarshalIndent(endpoints, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}
