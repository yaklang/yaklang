package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// JavaFrameworkDetector is the result of Phase 1 framework detection.
type JavaFrameworkDetector struct {
	SchemaVersion int             `json:"schema_version"`
	GeneratedAt  string         `json:"generated_at"`
	CodeRoot     string         `json:"code_root"`
	Frameworks   []FrameworkInfo `json:"frameworks"`
	IsJava       bool           `json:"is_java"`
	BuildSystem  string         `json:"build_system"` // maven, gradle, unknown
	Warnings     []string       `json:"warnings,omitempty"`
}

// BuildSystem detection regular expressions.
var (
	rePomXML     = regexp.MustCompile(`(?i)^pom\.xml$`)
	reBuildGradle = regexp.MustCompile(`(?i)^build\.gradle(\.kts)?$`)
	reMavenWrapper = regexp.MustCompile(`(?i)^mvnw$`)
	reGradleWrapper = regexp.MustCompile(`(?i)^gradlew$`)

	// Maven dependency patterns
	reMavenDepGroup    = regexp.MustCompile(`<groupId>\s*([^<]+)\s*</groupId>`)
	reMavenDepArtifact = regexp.MustCompile(`<artifactId>\s*([^<]+)\s*</artifactId>`)

	// Gradle dependency patterns
	reGradleDep = regexp.MustCompile(`(?:implementation|compile|api)\s+['"]([^'":]+):([^'":]+)(?::([^'"]+))?['"]`)

	// Spring detection
	reSpringWebAnnotation = regexp.MustCompile(`@(?:Rest)?Controller\b|@RequestMapping\b|@(?:Get|Post|Put|Delete|Patch)Mapping\b`)

	// JAX-RS detection
	reJAXRSImport = regexp.MustCompile(`(?i)(jakarta\.ws\.rs|javax\.ws\.rs)\b`)
	reJAXRSPPath  = regexp.MustCompile(`@Path\s*\(\s*["']([^"']+)["']`)
	reJAXRSMethod = regexp.MustCompile(`@(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s*(?:\(|;|\{)`)

	// Servlet detection
	reServletImport = regexp.MustCompile(`(?i)(jakarta\.servlet|javax\.servlet)\b`)
)

// DetectJavaBuildSystem detects whether this is a Maven or Gradle project.
func DetectJavaBuildSystem(codeRoot string) (buildSystem string, pomContent, gradleContent string) {
	mavenFiles := []string{"pom.xml"}
	gradleFiles := []string{"build.gradle", "build.gradle.kts"}

	// Check for Maven
	for _, name := range mavenFiles {
		pomPath := filepath.Join(codeRoot, name)
		if data, err := os.ReadFile(pomPath); err == nil {
			pomContent = string(data)
			return "maven", pomContent, ""
		}
	}

	// Check for Gradle
	for _, name := range gradleFiles {
		gradlePath := filepath.Join(codeRoot, name)
		if data, err := os.ReadFile(gradlePath); err == nil {
			gradleContent = string(data)
			return "gradle", "", gradleContent
		}
	}

	// Check for wrappers
	files, _ := os.ReadDir(codeRoot)
	for _, f := range files {
		name := strings.ToLower(f.Name())
		if reMavenWrapper.MatchString(name) {
			return "maven", "", ""
		}
		if reGradleWrapper.MatchString(name) {
			return "gradle", "", ""
		}
	}

	return "unknown", "", ""
}

// RunJavaFrameworkDetection scans a Java project and detects its framework(s).
func RunJavaFrameworkDetection(rt *Runtime) (*JavaFrameworkDetector, error) {
	if rt == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	if !rt.Session.CodePathOK {
		return nil, utils.Error("code path not ok")
	}

	codeRoot := rt.Session.CodeRootPath
	detector := &JavaFrameworkDetector{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		CodeRoot:      codeRoot,
		Frameworks:    []FrameworkInfo{},
		IsJava:        false,
		BuildSystem:   "unknown",
	}

	// Step 1: Detect build system
	buildSystem, pomContent, gradleContent := DetectJavaBuildSystem(codeRoot)
	detector.BuildSystem = buildSystem
	if buildSystem == "unknown" {
		detector.Warnings = append(detector.Warnings, "No Maven or Gradle build file detected")
		return detector, nil
	}

	// Step 2: Collect Java source files for annotation scanning
	var javaFiles []string
	var javaFilesForScan []string
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
			if len(javaFiles) <= 500 {
				javaFilesForScan = append(javaFilesForScan, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(javaFiles) == 0 {
		detector.Warnings = append(detector.Warnings, "No Java source files found")
		return detector, nil
	}
	detector.IsJava = true

	// Step 3: Scan build file for dependencies
	depContent := pomContent + gradleContent

	// Step 4: Scan Java files for annotations (only first 500 for performance)
	var javaContentForAnnotation string
	for _, f := range javaFilesForScan {
		if data, err := os.ReadFile(f); err == nil {
			javaContentForAnnotation += string(data) + "\n"
		}
	}

	// Step 5: Detect frameworks
	detected := DetectFrameworksFromContent(depContent, javaContentForAnnotation)
	detector.Frameworks = detected

	// Log results
	var frameworkNames []string
	for _, f := range detected {
		frameworkNames = append(frameworkNames, string(f.Type))
	}
	log.Infof("ssa_api_discovery: java_framework_detection frameworks=%v build_system=%s java_files=%d",
		frameworkNames, buildSystem, len(javaFiles))

	// Persist the result
	if err := persistFrameworkDetector(rt, detector); err != nil {
		log.Warnf("ssa_api_discovery: failed to persist framework detector: %v", err)
	}

	return detector, nil
}

// DetectFrameworksFromContent analyzes build and source content to detect frameworks.
func DetectFrameworksFromContent(buildContent, javaContent string) []FrameworkInfo {
	var results []FrameworkInfo
	lower := strings.ToLower(buildContent)

	// --- Spring Boot Detection ---
	springScore := detectSpringScore(lower, buildContent, javaContent)
	if springScore >= 0.3 {
		results = append(results, FrameworkInfo{
			Type:       FrameworkSpringBoot,
			Confidence: minFloat(springScore, 1.0),
			Evidence:   extractSpringEvidence(lower, javaContent),
		})
	}

	// --- JAX-RS Detection ---
	jaxrsScore := detectJAXRSScore(lower, javaContent)
	if jaxrsScore >= 0.3 {
		results = append(results, FrameworkInfo{
			Type:       FrameworkJAXRS,
			Confidence: minFloat(jaxrsScore, 1.0),
			Evidence:   "JAX-RS API detected in dependencies or source",
		})
	}

	// --- Struts 2 Detection ---
	if strings.Contains(lower, "struts") && !strings.Contains(lower, "struts-security") {
		// Check if it's actually Struts 2 (not just a mention in docs)
		if strings.Contains(lower, "struts2") || strings.Contains(lower, "struts-2") ||
			strings.Contains(lower, "struts2-core") || strings.Contains(javaContent, "@Action") {
			results = append(results, FrameworkInfo{
				Type:       FrameworkStruts2,
				Confidence: 0.7,
				Evidence:   "Apache Struts 2 dependency or annotations found",
			})
		}
	}

	// --- Servlet Detection (always baseline) ---
	if strings.Contains(lower, "servlet") || strings.Contains(javaContent, "extends HttpServlet") {
		results = append(results, FrameworkInfo{
			Type:       FrameworkServlet,
			Confidence: 0.5,
			Evidence:   "Java Servlet API detected",
		})
	}

	return results
}

// detectSpringScore calculates a confidence score for Spring Boot.
func detectSpringScore(lower, buildContent, javaContent string) float64 {
	score := 0.0

	// Build file indicators
	if strings.Contains(lower, "spring-boot-starter-web") {
		score += 0.5
	}
	if strings.Contains(lower, "spring-boot-starter") {
		score += 0.3
	}
	if strings.Contains(lower, "spring-web") || strings.Contains(lower, "springframework") {
		score += 0.2
	}
	if strings.Contains(lower, "spring-boot-starter-security") {
		score += 0.1
	}

	// Annotation indicators
	if reSpringBootApp.MatchString(javaContent) {
		score += 0.3
	}
	if reController.MatchString(javaContent) {
		score += 0.2
	}
	if strings.Contains(javaContent, "@GetMapping") || strings.Contains(javaContent, "@PostMapping") {
		score += 0.2
	}
	if strings.Contains(javaContent, "@RequestMapping") {
		score += 0.15
	}

	return score
}

// detectJAXRSScore calculates a confidence score for JAX-RS.
func detectJAXRSScore(lower, javaContent string) float64 {
	score := 0.0

	if strings.Contains(lower, "jakarta.ws.rs") {
		score += 0.5
	}
	if strings.Contains(lower, "javax.ws.rs") {
		score += 0.4
	}
	if strings.Contains(lower, "jersey") {
		score += 0.2
	}
	if strings.Contains(lower, "resteasy") {
		score += 0.2
	}
	if strings.Contains(lower, "quarkus-rest") || strings.Contains(lower, "quarkus-resteasy") {
		score += 0.3
	}

	// Annotation indicators
	if reJAXRSPPath.MatchString(javaContent) {
		score += 0.3
	}
	if reJAXRSImport.MatchString(javaContent) {
		score += 0.2
	}

	return score
}

// extractSpringEvidence extracts evidence for Spring detection.
func extractSpringEvidence(lower, javaContent string) string {
	if strings.Contains(lower, "spring-boot-starter-web") {
		return "spring-boot-starter-web dependency found"
	}
	if strings.Contains(lower, "spring-boot") {
		return "Spring Boot dependency found"
	}
	if reSpringBootApp.MatchString(javaContent) {
		return "@SpringBootApplication annotation found"
	}
	if reController.MatchString(javaContent) {
		return "@Controller or @RestController annotation found"
	}
	return "Spring Framework indicators found"
}

// persistFrameworkDetector saves the detector result.
func persistFrameworkDetector(rt *Runtime, detector *JavaFrameworkDetector) error {
	if rt == nil || detector == nil {
		return utils.Error("nil detector")
	}
	b, err := json.MarshalIndent(detector, "", "  ")
	if err != nil {
		return err
	}
	path := store.JavaFrameworkDetectorPath(rt.WorkDir)
	if err := writeJSONFile(path, b); err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactJavaFrameworkDetector, string(b))
	}
	return nil
}

// loadFrameworkDetector loads a saved detector result.
func loadFrameworkDetector(workDir string) (*JavaFrameworkDetector, error) {
	path := store.JavaFrameworkDetectorPath(workDir)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var detector JavaFrameworkDetector
	if err := json.Unmarshal(b, &detector); err != nil {
		return nil, err
	}
	return &detector, nil
}

// HasFramework checks if a specific framework was detected.
func (d *JavaFrameworkDetector) HasFramework(frameworkType JavaFrameworkType) bool {
	for _, f := range d.Frameworks {
		if f.Type == frameworkType {
			return true
		}
	}
	return false
}

// GetFramework returns the framework info for a specific type.
func (d *JavaFrameworkDetector) GetFramework(frameworkType JavaFrameworkType) *FrameworkInfo {
	for _, f := range d.Frameworks {
		if f.Type == frameworkType {
			return &f
		}
	}
	return nil
}

// PrimaryFramework returns the most likely framework (highest confidence).
func (d *JavaFrameworkDetector) PrimaryFramework() *FrameworkInfo {
	if len(d.Frameworks) == 0 {
		return nil
	}
	best := &d.Frameworks[0]
	for i := 1; i < len(d.Frameworks); i++ {
		if d.Frameworks[i].Confidence > best.Confidence {
			best = &d.Frameworks[i]
		}
	}
	return best
}

// AddFramework adds a framework detection result.
func (d *JavaFrameworkDetector) AddFramework(frameworkType JavaFrameworkType, confidence float64, evidence string) {
	// Check if already exists
	for i, f := range d.Frameworks {
		if f.Type == frameworkType {
			if confidence > f.Confidence {
				d.Frameworks[i].Confidence = confidence
				d.Frameworks[i].Evidence = evidence
			}
			return
		}
	}
	d.Frameworks = append(d.Frameworks, FrameworkInfo{
		Type:       frameworkType,
		Confidence: confidence,
		Evidence:   evidence,
	})
}
