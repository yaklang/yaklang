package analyzer

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	licenses "github.com/yaklang/yaklang/common/sca/license"
)

const (
	TypAPK TypAnalyzer = "apk-pkg"

	// installed file
	installFile           = "lib/apk/db/installed"
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
			license[i-1] = licenses.Normalize(license[i-1] + s)
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
		version = name[strings.IndexAny(name, "><=")+1:]
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

func (a apkAnalyzer) handleDependsOn(pkgs []dxtypes.Package, provides map[string]*dxtypes.Package) {
	for i := range pkgs {
		// e.g. libc6 => libc6@2.31-13+deb11u4
		pkg := &pkgs[i]

		ret := make(map[string]string)
		for provide_name, version := range pkg.DependsOn.And {
			if p, ok := provides[provide_name]; ok {
				ret[p.Name] = p.Version
			}
			if oldVersion, ok := ret[provide_name]; !ok || fastVersionCompare(version, oldVersion) {
				// ret[packageName] = packageVersion
				ret[provide_name] = version
			}
		}

		if len(pkg.DependsOn.And) == 0 {
			pkg.DependsOn.And = nil
		}
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

		provides := make(map[string]*dxtypes.Package)
		pkg = new(dxtypes.Package)

		scanner := bufio.NewScanner(fi.LazyFile)
		for scanner.Scan() {
			line := scanner.Text()

			if len(line) < 2 {
				if pkg.Name != "" && pkg.Version != "" {
					pkgs = append(pkgs, pkg)
				}
				// new
				// pkg = &dxtypes.Package{}
				pkg = new(dxtypes.Package)
				continue
			}
			// ref. https://wiki.alpinelinux.org/wiki/Apk_spec
			switch line[:2] {
			case "P:":
				pkg.Name = line[2:]
			case "V:":
				version = line[2:]
				pkg.Version = version
			case "L:":
				pkg.License = a.parseLicense(line)
			case "p:":
				a.parseProvides(line, pkg, provides)
			case "D:": // dependencies (corresponds to depend in PKGINFO, concatenated by spaces into a single line)
				pkg.DependsOn.And = a.parseDependencies(line)
			case "C:":
				pkg.Verification = decodeChecksumLine(line)
			}
		}
		if pkg.Name != "" && pkg.Version != "" {
			pkgs = append(pkgs, pkg)
		}

		handleDependsOn(pkgs, provides)

		return linkUpSteamAndDownStream(pkgs), nil
	}
	return nil, nil
}

func (a apkAnalyzer) Match(info MatchInfo) int {
	if info.path == installFile {
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
