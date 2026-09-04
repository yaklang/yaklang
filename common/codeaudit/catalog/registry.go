package catalog

// registry.go is the per-language dispatch hub. Adding a language means
// adding a content pack (a catalog file with framework signals plus a
// config-rule file); everything routed here stays mechanical.

// FrameworkCatalog returns the framework signal catalog for a language.
// Unknown languages fall back to the Java catalog (the historical default).
func FrameworkCatalog(language string) []FrameworkSignal {
	switch language {
	case "python":
		return PythonFrameworkCatalog
	case "go":
		return GoFrameworkCatalog
	case "php":
		return PHPFrameworkCatalog
	case "node":
		return NodeFrameworkCatalog
	default:
		return JavaFrameworkCatalog
	}
}

// CmsCatalog returns the CMS fingerprint catalog for a language. Languages
// without known product fingerprints return nil.
func CmsCatalog(language string) []CmsFingerprint {
	switch language {
	case "java":
		return JavaCmsCatalog
	case "php":
		return PHPCmsCatalog
	default:
		return nil
	}
}

// AllConfigRules returns every config rule registered for a language.
func AllConfigRules(language string) []ConfigCheckRule {
	switch language {
	case "python":
		return PythonConfigRules
	case "go":
		return GoConfigRules
	case "php":
		return PHPConfigRules
	case "node":
		return NodeConfigRules
	default:
		return JavaConfigRules
	}
}

// GetConfigRules returns the config rules of a language filtered by framework.
func GetConfigRules(language, framework string) []ConfigCheckRule {
	var out []ConfigCheckRule
	for _, r := range AllConfigRules(language) {
		if r.Framework == framework {
			out = append(out, r)
		}
	}
	return out
}

// GetCmsConfigRules returns config rules for CMS-specific checks. These are
// rules that apply regardless of framework when a CMS product is detected.
func GetCmsConfigRules(language, cmsID string) []ConfigCheckRule {
	switch cmsID {
	case "ruoyi", "ruoyi-cloud":
		return []ConfigCheckRule{
			{
				Framework:      "ruoyi",
				ID:             "java.ruoyi.password.plain",
				Severity:       "high",
				Title:          "RuoYi database password in plain text",
				Recommendation: "move database password to environment variable or encrypted config",
				FilePatterns:   []string{"application*.yml", "application*.yaml", "application*.properties"},
				MaskValue:      true,
			},
		}
	case "wordpress":
		return []ConfigCheckRule{
			{
				Framework:      "wordpress",
				ID:             "php.wordpress.db_password",
				Severity:       "high",
				Title:          "WordPress database password in plain text",
				Recommendation: "move DB_PASSWORD out of wp-config.php into environment variables or a secrets manager",
				FilePatterns:   []string{"wp-config*.php"},
				MaskValue:      true,
			},
		}
	default:
		return nil
	}
}

// BuildSystemRule maps a marker file to a build system label. Rules are
// ordered: distinct labels among present markers mean "mixed".
type BuildSystemRule struct {
	Marker string // exact file name
	Name   string // build system label
}

// LanguageProfile captures the per-language file conventions shared by the
// probe, CMS fingerprinting and dependency scanners.
type LanguageProfile struct {
	// SourceExt is the primary source extension used as the content-marker
	// fallback when a framework signal has no file markers of its own.
	SourceExt string
	// CMSContentFiles lists exact file names whose content is checked for
	// CMS fingerprints.
	CMSContentFiles []string
	// DepManifests lists the dependency manifests counted as scanned files
	// by the dependency report.
	DepManifests []string
	// BuildSystems maps marker files to build system labels.
	BuildSystems []BuildSystemRule
}

// LanguageProfileFor returns the file conventions for a language. Unknown
// languages fall back to the Java profile.
func LanguageProfileFor(language string) LanguageProfile {
	switch language {
	case "python":
		return LanguageProfile{
			SourceExt:       ".py",
			CMSContentFiles: []string{"requirements.txt", "manage.py", "settings.py"},
			DepManifests:    []string{"requirements.txt", "pyproject.toml", "Pipfile", "poetry.lock", "setup.py"},
			BuildSystems: []BuildSystemRule{
				{Marker: "poetry.lock", Name: "poetry"},
				{Marker: "Pipfile", Name: "pipenv"},
				{Marker: "requirements.txt", Name: "pip"},
				{Marker: "pyproject.toml", Name: "pip"},
				{Marker: "setup.py", Name: "setuptools"},
			},
		}
	case "go":
		return LanguageProfile{
			SourceExt:       ".go",
			CMSContentFiles: []string{"go.mod"},
			DepManifests:    []string{"go.mod", "go.sum"},
			BuildSystems: []BuildSystemRule{
				{Marker: "go.mod", Name: "go-modules"},
			},
		}
	case "php":
		return LanguageProfile{
			SourceExt:       ".php",
			CMSContentFiles: []string{"composer.json"},
			DepManifests:    []string{"composer.json", "composer.lock"},
			BuildSystems: []BuildSystemRule{
				{Marker: "composer.json", Name: "composer"},
			},
		}
	case "node":
		return LanguageProfile{
			SourceExt:       ".js",
			CMSContentFiles: []string{"package.json"},
			DepManifests:    []string{"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml"},
			BuildSystems: []BuildSystemRule{
				{Marker: "pnpm-lock.yaml", Name: "pnpm"},
				{Marker: "yarn.lock", Name: "yarn"},
				{Marker: "package-lock.json", Name: "npm"},
				{Marker: "package.json", Name: "npm"},
			},
		}
	default:
		return LanguageProfile{
			SourceExt:       ".java",
			CMSContentFiles: []string{"pom.xml", "build.gradle", "application"},
			DepManifests:    []string{"pom.xml", "build.gradle"},
			BuildSystems: []BuildSystemRule{
				{Marker: "pom.xml", Name: "maven"},
				{Marker: "build.gradle", Name: "gradle"},
				{Marker: "build.gradle.kts", Name: "gradle"},
			},
		}
	}
}
