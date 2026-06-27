package loop_ssa_api_discovery

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const springEndpointHarvestWorkers = 8

// Spring endpoint extraction regular expressions.
var (
	// Class-level annotations
	reSpringRestController = regexp.MustCompile(`@(?:Rest)?Controller\b`)
	reSpringController    = regexp.MustCompile(`@Controller\b`)
	reSpringRequestMapping = regexp.MustCompile(`@RequestMapping\s*\(`)

	// Method-level mappings
	reSpringGetMapping    = regexp.MustCompile(`@GetMapping\s*(?:\(\s*)?`)
	reSpringPostMapping   = regexp.MustCompile(`@PostMapping\s*(?:\(\s*)?`)
	reSpringPutMapping    = regexp.MustCompile(`@PutMapping\s*(?:\(\s*)?`)
	reSpringDeleteMapping = regexp.MustCompile(`@DeleteMapping\s*(?:\(\s*)?`)
	reSpringPatchMapping  = regexp.MustCompile(`@PatchMapping\s*(?:\(\s*)?`)

	// RequestMapping with various attributes
	reSpringReqMapValue  = regexp.MustCompile(`@RequestMapping\s*\(\s*(?:value|path)\s*=\s*["']([^"']+)["']`)
	reSpringReqMapMethod = regexp.MustCompile(`method\s*=\s*RequestMethod\.(GET|POST|PUT|DELETE|PATCH)`)

	// Shortcut mapping variants - handle both @GetMapping("path") and @GetMapping(value="path")
	reSpringMappingValue = regexp.MustCompile(`@(?:Get|Post|Put|Delete|Patch|Request)Mapping\s*\(\s*(?:value\s*=\s*)?["']([^"']+)["']`)
	reSpringMappingBare  = regexp.MustCompile(`@(?:Get|Post|Put|Delete|Patch|Request)Mapping\s*\(\s*\)`)

	// Path variable extraction
	rePathVariable = regexp.MustCompile(`\{([^}]+)\}`)

	// Request parameter annotations
	reRequestParam  = regexp.MustCompile(`@RequestParam\s*(?:\(\s*(?:value|name)?\s*=\s*)?["']?(\w+)["']?\s*(?:\)|,)`)
	rePathParam     = regexp.MustCompile(`@PathVariable\s*(?:\(\s*(?:value|name)?\s*=\s*)?["']?(\w+)["']?\s*(?:\)|,)`)
	reRequestBody   = regexp.MustCompile(`@RequestBody\s*(?:\(\s*)?`)
	reRequestHeader = regexp.MustCompile(`@RequestHeader\s*(?:\(\s*(?:value|name)?\s*=\s*)?["']?(\w+)["']?\s*(?:\)|,)`)

	// Security annotations
	reSpringPreAuthorize   = regexp.MustCompile(`@PreAuthorize\s*\(\s*["']([^"']+)["']\s*\)`)
	reSpringSecured        = regexp.MustCompile(`@Secured\s*\(\s*\{?\s*["']([^"']+)["']`)
	reSpringRolesAllowed   = regexp.MustCompile(`@RolesAllowed\s*\(\s*\{?\s*["']([^"']+)["']`)
	reSpringPermitAll      = regexp.MustCompile(`@PermitAll\s*(?:\(|;|\{|\s|$)`)
	reSpringDenyAll        = regexp.MustCompile(`@DenyAll\s*(?:\(|;|\{|\s|$)`)
	reSpringAnonymous      = regexp.MustCompile(`@Anonymous\s*(?:\(|;|\{|\s|$)`)

	// Validation annotations
	reNotNull     = regexp.MustCompile(`@NotNull\b`)
	reNotBlank    = regexp.MustCompile(`@NotBlank\b`)
	reNotEmpty    = regexp.MustCompile(`@NotEmpty\b`)
	reSize        = regexp.MustCompile(`@Size\s*\(\s*(?:min\s*=\s*)?(\d+)`)
	reMin         = regexp.MustCompile(`@Min\s*\(\s*value\s*=\s*(\d+)`)
	reMax         = regexp.MustCompile(`@Max\s*\(\s*value\s*=\s*(\d+)`)
	rePattern     = regexp.MustCompile(`@Pattern\s*\(\s*regexp\s*=\s*["']([^"']+)["']`)
	reValidated   = regexp.MustCompile(`@Validated\b`)

	// Deprecation
	reDeprecated  = regexp.MustCompile(`@Deprecated\s*(?:\(|;|\{|\s|$)`)
)

// SpringEndpointHarvester extracts Spring Boot/MVC endpoints.
type SpringEndpointHarvester struct {
	CodeRoot string
	Endpoints []APIEndpoint
	mu       sync.Mutex
}

// HarvestSpringEndpoints extracts all Spring endpoints from a Java project.
func HarvestSpringEndpoints(codeRoot string) ([]APIEndpoint, error) {
	h := &SpringEndpointHarvester{
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

	// Concurrent processing
	jobs := make(chan string, len(javaFiles))
	var wg sync.WaitGroup
	n := springEndpointHarvestWorkers
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

	// Deduplicate and compute stats
	return deduplicateEndpoints(h.Endpoints), nil
}

// extractFromFile extracts endpoints from a single Java file.
func (h *SpringEndpointHarvester) extractFromFile(absPath string) []APIEndpoint {
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
	var classAnnotations []string // Store class-level annotations separately

	flushClassAnnotations := func() {
		// Save class annotations for later use
		classAnnotations = append(classAnnotations[:0], pendingAnnotations...)
		classBasePath = ""
		for _, a := range classAnnotations {
			classBasePath = springJoinPath(classBasePath, extractSpringPathValue(a))
		}
		// Clear pending annotations - class-level ones are now in classAnnotations
		pendingAnnotations = pendingAnnotations[:0]
	}

	extractMethodEndpoints := func(methodLine string, methodLineNum int) {
		if className == "" {
			return
		}

		methodName := extractJavaMethodName(methodLine)
		if methodName == "" {
			return
		}

		fqClass := fqClassName(packageName, className)

		// Extract HTTP methods and paths from annotations
		// Combine class-level annotations with method-level annotations
		allAnnotations := append(classAnnotations, pendingAnnotations...)
		var httpMethods []string
		var methodPaths []string
		var authRules []AuthRule

		for _, anno := range allAnnotations {
			anno = strings.TrimSpace(anno)

			// Extract HTTP method
			switch {
			case strings.Contains(anno, "@GetMapping"):
				httpMethods = append(httpMethods, "GET")
			case strings.Contains(anno, "@PostMapping"):
				httpMethods = append(httpMethods, "POST")
			case strings.Contains(anno, "@PutMapping"):
				httpMethods = append(httpMethods, "PUT")
			case strings.Contains(anno, "@DeleteMapping"):
				httpMethods = append(httpMethods, "DELETE")
			case strings.Contains(anno, "@PatchMapping"):
				httpMethods = append(httpMethods, "PATCH")
			case strings.Contains(anno, "@RequestMapping"):
				httpMethods = append(httpMethods, extractSpringRequestMethods(anno)...)
			}

			// Extract path
			if path := extractSpringPathValue(anno); path != "" {
				methodPaths = append(methodPaths, path)
			}

			// Extract auth rules
			authRules = append(authRules, extractAuthRules(anno)...)
		}

		// Default to empty paths if no explicit path
		if len(methodPaths) == 0 {
			methodPaths = []string{""}
		}

		// Default HTTP method for RequestMapping without method specified
		if len(httpMethods) == 0 && containsRequestMapping(allAnnotations) {
			httpMethods = []string{"GET"} // Conservative default
		}

		// Generate endpoint for each (method, path) combination
		for _, httpMethod := range httpMethods {
			for _, methodPath := range methodPaths {
				fullPath := springJoinPath(classBasePath, methodPath)

				// Extract path variables
				pathVars := extractPathVariables(fullPath)

				// Extract query parameters
				queryParams := extractQueryParams(methodLine)

				// Extract request body type
				var reqBody *RequestBody
				if strings.Contains(methodLine, "@RequestBody") || strings.Contains(methodLine, "@RequestPart") {
					reqBody = &RequestBody{
						ContentType: "application/json",
						TypeName:    extractParameterType(methodLine, "@RequestBody"),
					}
				}

		// Determine if auth is required
		authRequired := !isPermitAll(allAnnotations) && !isDenyAll(allAnnotations) &&
			(len(authRules) > 0 || containsSecurityConfig(allAnnotations))

				// Generate unique ID
				id := generateEndpointID(fqClass, methodName, httpMethod, fullPath)

				// RawAnnotations includes both class-level and method-level
				rawAnnotations := append(classAnnotations, pendingAnnotations...)

				ep := APIEndpoint{
					ID:              id,
					Framework:       FrameworkSpringBoot,
					ClassName:       fqClass,
					SimpleClassName: className,
					MethodName:      methodName,
					PackageName:     packageName,
					HTTPPath:        fullPath,
					HTTPMethods:     []string{httpMethod},
					PathVariables:   pathVars,
					QueryParams:     queryParams,
					RequestBody:     reqBody,
					AuthRequirements: authRules,
					AuthRequired:   authRequired,
					FilePath:        relPath,
					LineNumber:      methodLineNum,
					Confidence:      computeEndpointConfidence(authRules, pathVars, reqBody),
					Provenance:      "annotation",
					RawAnnotations:  rawAnnotations,
					Deprecated:      isDeprecated(rawAnnotations),
					SessionAttributeMethod: strings.Contains(methodLine, "@SessionAttribute"),
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

		// Track block comments
		if strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*") {
			continue
		}

		// Annotation line
		if strings.HasPrefix(t, "@") {
			pendingAnnotations = append(pendingAnnotations, t)
			pendingLineNum = currentLine
			continue
		}

		// Class declaration
		if reClassDecl.MatchString(t) {
			flushClassAnnotations()
			className = extractClassName(t)
			if m := reJavaPackage.FindSubmatch(data); len(m) > 1 {
				packageName = string(m[1])
			}
			continue
		}

		// Method declaration
		if isPublicMethod(t) && strings.Contains(t, "(") {
			extractMethodEndpoints(t, pendingLineNum)
			continue
		}

		// Reset annotations on other declarations
		if !strings.HasPrefix(t, "@") && len(pendingAnnotations) > 0 {
			if !strings.Contains(t, "class ") && !strings.Contains(t, "interface ") && !strings.Contains(t, "enum ") {
				pendingAnnotations = pendingAnnotations[:0]
			}
		}
	}

	return endpoints
}

// Helper functions.

func extractSpringPathValue(anno string) string {
	// Try @GetMapping("/path")
	if m := reSpringMappingValue.FindStringSubmatch(anno); len(m) > 1 {
		return m[1]
	}

	// Try @RequestMapping(value="/path") or @RequestMapping(path="/path")
	if m := reSpringReqMapValue.FindStringSubmatch(anno); len(m) > 1 {
		return m[1]
	}

	return ""
}

// extractSpringRequestMethods extracts HTTP methods from a Spring @RequestMapping annotation.
func extractSpringRequestMethods(anno string) []string {
	methods := reSpringReqMapMethod.FindAllStringSubmatch(anno, -1)
	if len(methods) == 0 {
		return nil
	}
	var result []string
	seen := make(map[string]bool)
	for _, m := range methods {
		if len(m) > 1 {
			method := strings.ToUpper(m[1])
			if !seen[method] {
				seen[method] = true
				result = append(result, method)
			}
		}
	}
	return result
}

func containsRequestMapping(annos []string) bool {
	for _, a := range annos {
		if strings.Contains(a, "@RequestMapping") {
			return true
		}
	}
	return false
}

func extractAuthRules(anno string) []AuthRule {
	var rules []AuthRule

	if m := reSpringPreAuthorize.FindStringSubmatch(anno); len(m) > 1 {
		roles := extractRolesFromExpression(m[1])
		rules = append(rules, AuthRule{
			Type:       "pre_authz",
			Expression: m[1],
			Roles:      roles,
		})
	}

	if m := reSpringSecured.FindStringSubmatch(anno); len(m) > 1 {
		roles := splitRoles(m[1])
		rules = append(rules, AuthRule{
			Type:  "pre_authz",
			Roles: roles,
		})
	}

	if m := reSpringRolesAllowed.FindStringSubmatch(anno); len(m) > 1 {
		roles := splitRoles(m[1])
		rules = append(rules, AuthRule{
			Type:  "pre_authz",
			Roles: roles,
		})
	}

	if reSpringPermitAll.MatchString(anno) {
		rules = append(rules, AuthRule{
			Type: "permit_all",
		})
	}

	if reSpringDenyAll.MatchString(anno) {
		rules = append(rules, AuthRule{
			Type: "deny_all",
		})
	}

	return rules
}

func extractRolesFromExpression(expr string) []string {
	var roles []string

	// hasRole('ADMIN') or hasAuthority('ROLE_ADMIN')
	reHasRole := regexp.MustCompile(`has(?:Role|Authority)\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	for _, m := range reHasRole.FindAllStringSubmatch(expr, -1) {
		if len(m) > 1 {
			role := m[1]
			// Normalize ROLE_ prefix
			if !strings.HasPrefix(role, "ROLE_") {
				role = "ROLE_" + role
			}
			roles = append(roles, role)
		}
	}

	// @PreAuthorize("isAuthenticated()")
	if strings.Contains(expr, "isAuthenticated") || strings.Contains(expr, "isAnonymous") {
		roles = append(roles, "AUTHENTICATED")
	}

	return roles
}

func splitRoles(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "{} \t")
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

func isPermitAll(annos []string) bool {
	for _, a := range annos {
		if reSpringPermitAll.MatchString(a) || reSpringAnonymous.MatchString(a) {
			return true
		}
	}
	return false
}

func isDenyAll(annos []string) bool {
	for _, a := range annos {
		if reSpringDenyAll.MatchString(a) {
			return true
		}
	}
	return false
}

func containsSecurityConfig(annos []string) bool {
	for _, a := range annos {
		if strings.Contains(a, "@EnableWebSecurity") || strings.Contains(a, "@EnableMethodSecurity") {
			return true
		}
	}
	return false
}

func isDeprecated(annos []string) bool {
	for _, a := range annos {
		if reDeprecated.MatchString(a) {
			return true
		}
	}
	return false
}

func extractPathVariables(path string) []PathVariable {
	var vars []PathVariable
	for _, m := range rePathVariable.FindAllStringSubmatch(path, -1) {
		if len(m) > 1 {
			vars = append(vars, PathVariable{
				Name: m[1],
				Type: "String", // Type cannot be determined from path alone
			})
		}
	}
	return vars
}

func extractQueryParams(methodLine string) []QueryParam {
	var params []QueryParam

	// Extract @RequestParam annotations from method signature
	// This is simplified - full extraction would need method signature parsing
	for _, m := range reRequestParam.FindAllStringSubmatch(methodLine, -1) {
		if len(m) > 1 {
			params = append(params, QueryParam{
				Name:     m[1],
				Required: !strings.Contains(methodLine, "required=false"),
			})
		}
	}

	return params
}

func extractParameterType(methodLine, annotation string) string {
	// Simple extraction - look for type before annotation
	idx := strings.Index(methodLine, annotation)
	if idx <= 0 {
		return ""
	}
	before := strings.TrimSpace(methodLine[:idx])
	parts := strings.Fields(before)
	if len(parts) < 2 {
		return ""
	}
	// Last part before annotation is the parameter name, second to last is type
	typeName := parts[len(parts)-2]
	// Remove generics like List<String>
	if strings.Contains(typeName, "<") {
		typeName = strings.Split(typeName, "<")[0]
	}
	return typeName
}

func computeEndpointConfidence(authRules []AuthRule, pathVars []PathVariable, reqBody *RequestBody) float64 {
	confidence := 0.7 // Base confidence

	// Higher confidence if we have auth rules
	if len(authRules) > 0 {
		confidence += 0.1
	}

	// Higher confidence if we have path variables (specific endpoint)
	if len(pathVars) > 0 {
		confidence += 0.1
	}

	// Higher confidence if we have request body
	if reqBody != nil {
		confidence += 0.05
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func generateEndpointID(className, methodName, httpMethod, path string) string {
	data := className + "#" + methodName + "#" + httpMethod + "#" + path
	hash := sha1.Sum([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func isPublicMethod(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "public ")
}

func extractClassName(line string) string {
	if m := reClassDecl.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	return ""
}

func reJavaPackageFind(data []byte) []byte {
	return reJavaPackage.Find(data)
}

func fqClassName(pkg, class string) string {
	if pkg == "" {
		return class
	}
	return pkg + "." + class
}

func deduplicateEndpoints(endpoints []APIEndpoint) []APIEndpoint {
	// Map to keep the most specific endpoint for each method
	// Key: className#methodName#httpMethod
	best := make(map[string]APIEndpoint)
	
	for _, ep := range endpoints {
		key := ep.ClassName + "#" + ep.MethodName + "#" + ep.HTTPMethods[0]
		
		existing, exists := best[key]
		if !exists {
			// First time seeing this method - always keep it
			best[key] = ep
		} else {
			// Prefer the endpoint with more specific path (longer path)
			if len(ep.HTTPPath) > len(existing.HTTPPath) {
				best[key] = ep
			} else if len(ep.HTTPPath) == len(existing.HTTPPath) && ep.HTTPPath != "/" {
				// If same specificity and not root path, prefer the one with auth rules
				if len(ep.AuthRequirements) > len(existing.AuthRequirements) {
					best[key] = ep
				}
			}
		}
	}
	
	// Convert map to slice
	result := make([]APIEndpoint, 0, len(best))
	for _, ep := range best {
		result = append(result, ep)
	}
	
	return result
}

// ExportSpringEndpointsToFile saves endpoints to a JSON file.
func ExportSpringEndpointsToFile(endpoints []APIEndpoint, outputPath string) error {
	data, err := json.MarshalIndent(endpoints, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}
