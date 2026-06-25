package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// PublicCMSToolkit implements FrameworkToolkit for PublicCMS multi-servlet Java CMS.
type PublicCMSToolkit struct{}

func (PublicCMSToolkit) ID() string    { return "publiccms" }
func (PublicCMSToolkit) Label() string { return "PublicCMS" }

func (t PublicCMSToolkit) Detect(rt *Runtime) (float64, []string) {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return 0, nil
	}
	root := rt.Session.CodeRootPath
	var evidence []string
	score := 0.0
	add := func(s string, w float64) {
		evidence = append(evidence, s)
		score += w
	}
	if fileContainsAnyUnder(root, "pom.xml", "com.publiccms", "publiccms-core") {
		add("pom: com.publiccms / publiccms-core", 0.25)
	}
	if fileContainsAnyUnder(root, "build.gradle", "com.publiccms", "publiccms") {
		add("gradle: publiccms module", 0.2)
	}
	if findFileNamedUnder(root, "AdminInitializer.java") != "" {
		add("java: AdminInitializer", 0.2)
	}
	if findFileNamedUnder(root, "ApiInitializer.java") != "" {
		add("java: ApiInitializer", 0.15)
	}
	if dirExistsUnder(root, "com/publiccms/controller/admin") {
		add("package: com.publiccms.controller.admin", 0.2)
	}
	if score > 1 {
		score = 1
	}
	return score, evidence
}

func (t PublicCMSToolkit) AcquireCredentials(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) error {
	return acquirePublicCMSCredentials(ctx, invoker, rt)
}

func (t PublicCMSToolkit) ExtractAPIs(rt *Runtime) (*CombinedAPICatalog, error) {
	return RunCombinedProgrammaticAPIExtraction(rt)
}

func (t PublicCMSToolkit) VerifyAPIs(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, catalog *CombinedAPICatalog) (*ToolkitVerifyReport, error) {
	return runFrameworkToolkitBulkVerify(ctx, invoker, rt, catalog, "publiccms")
}

func (t PublicCMSToolkit) WriteGateArtifacts(rt *Runtime, catalog *CombinedAPICatalog, report *ToolkitVerifyReport) error {
	return writeFrameworkToolkitGateArtifacts(rt, catalog, report, "publiccms")
}

func fileContainsAnyUnder(root, basename string, subs ...string) bool {
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) != basename && !strings.HasSuffix(strings.ToLower(path), "/"+basename) {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		lower := strings.ToLower(string(b))
		for _, sub := range subs {
			if !strings.Contains(lower, strings.ToLower(sub)) {
				return nil
			}
		}
		found = true
		return filepath.SkipAll
	})
	return found
}

func findFileNamedUnder(root, name string) string {
	var hit string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == name {
			hit = path
			return filepath.SkipAll
		}
		return nil
	})
	return hit
}

func dirExistsUnder(root, relSuffix string) bool {
	relSuffix = strings.Trim(relSuffix, "/")
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ReplaceAll(path, "\\", "/"), "/"+relSuffix) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func bootstrapPublicCMSAuthArtifacts(rt *Runtime) error {
	now := time.Now().UTC().Format(time.RFC3339)
	surface := &AuthSurfaceMapV1{
		SchemaVersion: artifactV2SchemaVersion,
		GeneratedAt:   now,
		MultiAuth:     true,
		Surfaces: []AuthSurfaceEntry{
			{
				AuthRealm:         AuthRealmAdmin,
				URLSpace:          "admin",
				MountPrefix:       "/admin",
				PackagePatterns:   []string{"com.publiccms.controller.admin.*"},
				PathPrefixes:      []string{"/admin"},
				SessionMechanism:  "cookie",
				SessionFields:     []string{"PUBLICCMS_ADMIN", "JSESSIONID"},
				PasswordTransform: "sha512",
				LoginPostPath:     "/admin/login",
				LoginMethod:       "POST",
				ContentType:       "application/x-www-form-urlencoded",
				CodeEvidence:      []string{"framework_toolkit:publiccms"},
			},
			{
				AuthRealm:        AuthRealmWeb,
				URLSpace:         "web",
				MountPrefix:      "/",
				PackagePatterns:  []string{"com.publiccms.controller.web.*"},
				PathPrefixes:     []string{"/"},
				SessionMechanism: "cookie",
				SessionFields:    []string{"PUBLICCMS_USER", "JSESSIONID"},
				CodeEvidence:     []string{"framework_toolkit:publiccms"},
			},
			{
				AuthRealm:        AuthRealmAPI,
				URLSpace:         "api",
				MountPrefix:      "/api",
				PackagePatterns:  []string{"com.publiccms.controller.api.*"},
				PathPrefixes:     []string{"/api"},
				SessionMechanism: "cookie",
				CodeEvidence:     []string{"framework_toolkit:publiccms"},
			},
		},
	}
	if err := persistAuthSurfaceMap(rt, surface); err != nil {
		return err
	}
	return nil
}

func acquirePublicCMSCredentials(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	if err := bootstrapPublicCMSAuthArtifacts(rt); err != nil {
		log.Warnf("ssa_api_discovery: publiccms auth bootstrap: %v", err)
	}
	if !rt.Session.TargetReachable {
		_, _ = writeAuthState(rt, authStateNoAuthNeeded, "target unreachable; toolkit static catalog only")
		return nil
	}
	username, password := resolveUserCredentialsFromRuntime(rt)
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "Admin@2024!"
	}
	if msg, err := tryPublicCMSAdminLogin(ctx, invoker, rt, username, password); err != nil {
		return err
	} else if msg != "" {
		log.Infof("ssa_api_discovery: %s", msg)
	}
	if _, err := TryProgrammaticLoginProbe(ctx, invoker, rt, username, password); err != nil {
		log.Warnf("ssa_api_discovery: generic login probe: %v", err)
	}
	if !hasVerifiedAuthCredential(rt) {
		return utils.Error("publiccms toolkit: no verified auth_credentials after login attempts")
	}
	n, csrfWarns := PrefetchCsrfTokensForSession(ctx, invoker, rt)
	if n > 0 {
		log.Infof("ssa_api_discovery: publiccms csrf prefetch cached=%d", n)
	}
	for _, w := range csrfWarns {
		log.Warnf("ssa_api_discovery: csrf prefetch: %s", w)
	}
	cal := &AuthCalibrationV1{
		SchemaVersion: artifactV2SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		AllCalibrated: true,
		Realms: []AuthCalibrationRealm{
			{AuthRealm: AuthRealmAdmin, MountPrefix: "/admin", Calibrated: true, Detail: "framework_toolkit:publiccms login"},
		},
	}
	if err := persistAuthCalibration(rt, cal); err != nil {
		return err
	}
	_, _ = writeAuthState(rt, authStateSuccess, "framework_toolkit publiccms credentials acquired")
	return nil
}

func tryPublicCMSAdminLogin(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime, username, password string) (string, error) {
	if invoker == nil || rt == nil || rt.Repo == nil || rt.Session == nil {
		return "", nil
	}
	base := EffectiveTargetBaseURL(rt.Session)
	if base == "" {
		return "", nil
	}
	hashRes, err := transformCredentialGoParams("sha512", password, "", "", "", "hex", false)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"username": {username},
		"password": {hashRes.Output},
		"encoding": {"sha512"},
	}.Encode()
	loginURL := strings.TrimRight(base, "/") + "/admin/login"
	params := aitool.InvokeParams{
		"url":          loginURL,
		"method":       "POST",
		"content-type": "application/x-www-form-urlencoded",
		"post-params":  form,
	}
	params, _ = augmentDoHTTPParams(params)
	result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", params)
	if err != nil {
		return "", err
	}
	content := toolResultTextContent(result)
	if !loginProbeSuccessful(content) {
		return "", nil
	}
	cred := &store.AuthCredential{
		SessionID:  rt.Session.ID,
		AuthType:   "cookie_session",
		AuthRealm:  AuthRealmAdmin,
		Username:   username,
		Verified:   true,
		VerifyURL:  loginURL,
		Notes:      "framework_toolkit publiccms sha512 login",
	}
	if headersJSON := buildAuthHeadersJSONFromLoginResponse(content); headersJSON != "" {
		cred.HeadersJSON = headersJSON
		SyncCredentialHeaderFields(cred)
	}
	if cred.HeadersJSON == "" {
		return "", nil
	}
	if err := rt.Repo.CreateAuthCredential(cred); err != nil {
		return "", err
	}
	_, _ = captureCsrfFromHTTPResponse(rt, cred, loginURL, content)
	return fmt.Sprintf("publiccms admin login ok credential_id=%d", cred.ID), nil
}

func resolveUserCredentialsFromRuntime(rt *Runtime) (username, password string) {
	if rt == nil {
		return "", ""
	}
	user := strings.TrimSpace(rt.UserAuthUsername)
	pass := strings.TrimSpace(rt.UserAuthPassword)
	if user == "" && len(rt.UserAuthCredentialGroups) > 0 {
		for _, g := range rt.UserAuthCredentialGroups {
			for _, acc := range g.Accounts {
				if strings.TrimSpace(acc.Username) != "" {
					return acc.Username, acc.Password
				}
			}
		}
	}
	return user, pass
}
