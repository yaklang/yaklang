package analyzer

import (
	"github.com/aquasecurity/go-dep-parser/pkg/rust/cargo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypCargo TypAnalyzer = "cargo-lang"

	CargoLock = "Cargo.lock"
	CargoToml = "Cargo.toml"

	statusCargoLock int = 1
	statusCargoToml int = 1
)

func init() {
	RegisterAnalyzer(TypCargo, NewRustCargoAnalyzer())
}

type cargoAnalyzer struct{}

func NewRustCargoAnalyzer() *cargoAnalyzer {
	return &cargoAnalyzer{}
}

func (a cargoAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusCargoLock:
		// build pkgs
		pkgs, err := ParseLanguageConfiguration(fi, cargo.NewParser())
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a cargoAnalyzer) Match(info MatchInfo) int {
	// Skip `composer.lock` inside `vendor` folder
	if info.fi.Name() == CargoLock {
		return statusCargoLock
	}
	if info.fi.Name() == CargoToml {
		return statusCargoToml
	}
	return 0
}
