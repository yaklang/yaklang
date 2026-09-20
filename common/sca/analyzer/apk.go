package analyzer

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	licenses "github.com/yaklang/yaklang/common/sca/license"
)

const (
	TypAPK TypAnalyzer = "apk-pkg"

	// installed file
	installFile           = "lib/apk/db/installed"
	installFileName       = "installed"
	statusInstallFile int = 1
)

func init() {
	RegisterAnalyzer(TypAPK, NewApkAnalyzer())
}

type apkAnalyzer struct{}

func NewApkAnalyzer() *apkAnalyzer {
	return &apkAnalyzer{}
}

func (a apkAnalyzer) parseLicense(line string) []string {
	line = line[2:] // Remove "L:"
	if line == "" {
		return nil
	}
	var license []string
	// e.g. MPL 2.0 GPL2+ => {"MPL2.0", "GPL2+"}
	for i, s := range strings.Fields(line) {
		s = strings.Trim(s, "()")
		if s == "AND" || s == "OR" {
			continue
		} else if i > 0 && (s == "1.0" || s == "2.0" || s == "3.0") {
			if len(license) > 0 {
				license[len(license)-1] = licenses.Normalize(license[len(license)-1] + s)
			}
		} else {
			license = append(license, licenses.Normalize(s))
		}
	}
	return license
}

func trimRequirement(name string) (string, string) {
	// Trim version requirements
	// e.g.
	//   so:libssl.so.1.1=1.1 => so:libssl.so.1.1
	//   musl>=1.2 => musl

	version := "*"
	if strings.ContainsAny(name, "<>=") {
		version = name[strings.IndexAny(name, "><="):]
		name = name[:strings.IndexAny(name, "><=")]
	}
	return name, version
}

func (a apkAnalyzer) parseDependencies(line string) map[string]string {
	ret := make(map[string]string)
	for _, s := range strings.Fields(line[2:]) {
		if strings.HasPrefix(s, "!") {
			continue
		}
		name, version := trimRequirement(s)
		ret[name] = version
	}
	return ret
}

func (a apkAnalyzer) parseProvides(line string, pkg *dxtypes.Package, provides map[string]*dxtypes.Package) {
	for _, p := range strings.Fields(line[2:]) {
		name, _ := trimRequirement(p)
		provides[name] = pkg
	}
}

func (a apkAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusInstallFile:
		var (
			pkgs    []*dxtypes.Package
			pkg     *dxtypes.Package
			version string
		)

		pkg = &dxtypes.Package{PackageDetails: &dxtypes.PackageDetails{Evidence: "installed"}}

		scanner := bufio.NewScanner(fi.LazyFile)
		scanner.Buffer(make([]byte, 4096), budget.From(fi.LazyFile.Context()).Limits.MaxFieldBytes)
		for scanner.Scan() {
			if err := fi.LazyFile.Context().Err(); err != nil {
				return nil, err
			}
			line := scanner.Text()

			if len(line) < 2 {
				if pkg.Name != "" && pkg.Version != "" {
					pkgs = append(pkgs, pkg)
				}
				// new
				// pkg = &dxtypes.Package{}
				pkg = &dxtypes.Package{PackageDetails: &dxtypes.PackageDetails{Evidence: "installed"}}
				continue
			}
			// ref. https://wiki.alpinelinux.org/wiki/Apk_spec
			switch line[:2] {
			case "P:":
				pkg.Name = line[2:]
			case "A:":
				pkg.Architecture = line[2:]
			case "V:":
				version = line[2:]
				pkg.Version = version
			case "L:":
				pkg.License = a.parseLicense(line)
				pkg.RawLicenses = []string{line[2:]}
			case "D:": // dependencies (corresponds to depend in PKGINFO, concatenated by spaces into a single line)
				pkg.DependsOn.And = a.parseDependencies(line)
			case "p:":
				pkg.Provides = append(pkg.Provides, strings.Fields(line[2:])...)
			case "C:":
				pkg.Verification = decodeChecksumLine(line)
			}
		}
		if pkg.Name != "" && pkg.Version != "" {
			pkgs = append(pkgs, pkg)
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a apkAnalyzer) Match(info MatchInfo) int {
	_, filename := info.fileSystem.PathSplit(info.Path)
	if info.Path == installFile || filename == installFileName {
		return statusInstallFile
	}
	return 0
}

// decodeChecksumLine decodes checksum line
func decodeChecksumLine(line string) string {
	if len(line) < 2 {
		return ""
	}
	alg := ""
	// https://wiki.alpinelinux.org/wiki/Apk_spec#Package_Checksum_Field
	// https://stackoverflow.com/a/71712569
	d := line[2:]
	if strings.HasPrefix(d, "Q1") {
		alg += "sha1:"
		d = d[2:] // remove `Q1` prefix
	} else {
		alg += "md5:"
	}

	decodedDigestString, err := base64.StdEncoding.DecodeString(d)
	if err != nil {
		return ""
	}
	h := hex.EncodeToString(decodedDigestString)
	return alg + h
}
