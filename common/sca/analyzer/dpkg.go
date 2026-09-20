package analyzer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/textproto"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypDPKG TypAnalyzer = "dpkg-pkg"

	statusFile = "var/lib/dpkg/status"
	statusDir  = "var/lib/dpkg/status.d/"
	// availableFile = "var/lib/dpkg/available"
	// infoDir       = "var/lib/dpkg/info/"

	statusStatus int = 1
)

func init() {
	RegisterAnalyzer(TypDPKG, NewDpkgAnalyzer())
}

type dpkgAnalyzer struct{}

func (a dpkgAnalyzer) parseStatus(s string) bool {
	fields := strings.Fields(s)
	return len(fields) == 3 && fields[2] == "installed"
}

func (a dpkgAnalyzer) parseDepends(s string) dxtypes.PackageRelationShip {
	// e.g. passwd, debconf (>= 0.5) | debconf-2.0
	// var dependencies []string

	packageRelationShip := dxtypes.PackageRelationShip{}
	if s == "" {
		return packageRelationShip
	}
	depends := strings.Split(s, ",")
	// init packageRelationShip
	packageRelationShip.And = make(map[string]string, len(depends))
	packageRelationShip.Or = make([]map[string]string, 0)

	for _, depName := range depends {
		// e.g. gpgv | gpgv2 | gpgv1
		// Store only package names
		if strings.Contains(depName, "|") {
			depNameVersionMap := make(map[string]string)
			for _, d := range strings.Split(depName, "|") {
				dep, version := a.getPackageNameAndVersion(d)
				depNameVersionMap[strings.TrimSpace(dep)] = version
			}
			packageRelationShip.Or = append(packageRelationShip.Or, depNameVersionMap)
		} else {
			dep, version := a.getPackageNameAndVersion(depName)
			packageRelationShip.And[strings.TrimSpace(dep)] = version
		}
	}
	return packageRelationShip
}

func (a dpkgAnalyzer) getPackageNameAndVersion(raw string) (string, string) {
	// e.g.
	//	libapt-pkg6.0 (>= 2.2.4) => libapt-pkg6.0, >= 2.2.4
	//	adduser => adduser
	pkgName := strings.TrimSpace(raw)
	version := "*"
	if strings.Contains(raw, "(") {
		version = strings.TrimSuffix(
			pkgName[strings.Index(pkgName, "(")+1:],
			")",
		)
		pkgName = pkgName[:strings.Index(pkgName, "(")]
	}

	return pkgName, version
}

func (a dpkgAnalyzer) parseDpkgPkg(header textproto.MIMEHeader) *dxtypes.Package {
	status := header.Get("Status")
	if status == "" {
		return nil
	}
	if isInstalled := a.parseStatus(status); !isInstalled {
		return nil
	}

	pkg := &dxtypes.Package{
		Name: header.Get("Package"),

		Version:   header.Get("Version"),
		DependsOn: a.parseDepends(header.Get("Depends")), PackageDetails: &dxtypes.PackageDetails{Architecture: header.Get("Architecture"), Evidence: "installed"},
	}
	if pkg.Name == "" || pkg.Version == "" {
		return nil
	}
	for _, raw := range strings.Split(header.Get("Provides"), ",") {
		if v := strings.TrimSpace(raw); v != "" {
			pkg.Provides = append(pkg.Provides, v)
		}
	}

	return pkg
}

func (a dpkgAnalyzer) analyzeStatus(r io.Reader) ([]*dxtypes.Package, error) {
	pkgs := make([]*dxtypes.Package, 0)
	br := bufio.NewReader(r)
	ctx := types.ContextOf(r)
	limits := budget.From(ctx).Limits
	for {
		block, err := readBlock(ctx, br)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if len(block) == 0 {
			break
		}
		reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(block)))
		header, err := reader.ReadMIMEHeader()
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("parse MIME header error: %v ", err)
		}
		if header.Get("Status") == "" {
			return pkgs, fmt.Errorf("evidence_insufficient: DPKG record without Status")
		}
		if a.parseStatus(header.Get("Status")) {
			if header.Get("Package") == "" || header.Get("Version") == "" {
				return pkgs, fmt.Errorf("malformed_input: installed DPKG record without identity")
			}
			if len(pkgs) >= limits.MaxComponents {
				return pkgs, fmt.Errorf("resource_limit: DPKG records")
			}
		}
		pkg := a.parseDpkgPkg(header)
		if pkg != nil {
			if err := budget.From(ctx).Result(budget.SizeOfPackage(pkg.Name, pkg.Version, pkg.Architecture)); err != nil {
				return nil, err
			}
			pkgs = append(pkgs, pkg)
		}
	}

	return pkgs, nil
}

func NewDpkgAnalyzer() *dpkgAnalyzer {
	return &dpkgAnalyzer{}
}

func (a dpkgAnalyzer) Match(info MatchInfo) int {
	if strings.HasPrefix(info.Path, statusDir) || info.Path == statusFile {
		// handler status
		return statusStatus
	}
	return 0
}

func (a dpkgAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusStatus:
		return a.analyzeStatus(fi.LazyFile)
	}

	return nil, nil
}
