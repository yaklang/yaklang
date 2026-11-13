package sfpattern

import (
	"path"
	"strings"
)

// manifestBaseNames are dependency and tooling manifests: files that describe
// which packages a project uses, not what it does.
//
// A rule whose file filter is the blanket "*" reads code and configuration, so
// it must not read these. A package.json lists dependency names and scripts, so
// a credential pattern such as `password: "..."` reports a finding that no
// attacker can use; the manifest only names the dependency. Rules that really
// need a manifest list it by name ("*package.json"), and that filter still
// matches: the skip applies to the blanket filter only.
var manifestBaseNames = map[string]bool{
	// JavaScript / TypeScript
	"package.json":        true,
	"package-lock.json":   true,
	"npm-shrinkwrap.json": true,
	"yarn.lock":           true,
	"pnpm-lock.yaml":      true,
	"bower.json":          true,
	"tsconfig.json":       true,
	"jsconfig.json":       true,
	"manifest.json":       true,
	// PHP / Python
	"composer.json":  true,
	"composer.lock":  true,
	"pyproject.toml": true,
	"pipfile":        true,
	"pipfile.lock":   true,
	"poetry.lock":    true,
	// Rust / Ruby / Go / Dart
	"cargo.toml":   true,
	"cargo.lock":   true,
	"gemfile":      true,
	"gemfile.lock": true,
	"go.mod":       true,
	"go.sum":       true,
	"pubspec.yaml": true,
	"pubspec.lock": true,
	// Java build machinery
	"pom.xml":             true,
	"build.gradle":        true,
	"build.gradle.kts":    true,
	"settings.gradle":     true,
	"settings.gradle.kts": true,
	// .NET project files
	"packages.config":       true,
	"directory.build.props": true,
}

// manifestSuffixes are manifest families whose file name varies per project.
var manifestSuffixes = []string{".csproj", ".fsproj", ".vbproj", ".sln"}

// blanketFileExcluded reports whether a file is a manifest that the blanket
// file filter "*" must skip. See manifestBaseNames for the reasoning.
func blanketFileExcluded(file string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(file, `\`, "/")))
	if base == "" || base == "." || base == "/" {
		return false
	}
	if manifestBaseNames[base] {
		return true
	}
	for _, suffix := range manifestSuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}
