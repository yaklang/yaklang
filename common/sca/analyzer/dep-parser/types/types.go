package types

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/model"
	"io"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type ReadSeekerAt interface {
	io.ReadSeeker
	io.ReaderAt
}

type ReadSeekCloserAt interface {
	io.ReadSeekCloser
	io.ReaderAt
}

type Library struct {
	Condition, Scope                        string
	IsVersionRange                          bool
	DeclaredCondition, Extras, Verification string
	DeclaredIntegrity                       string
	Diagnostics                             []model.Diagnostic

	Source, Variant, Evidence, DeclaredName, DeclaredVersion string

	ID                 string `json:",omitempty"`
	Name               string
	Version            string
	Dev                bool
	Indirect           bool          `json:",omitempty"`
	License            string        `json:",omitempty"`
	ExternalReferences []ExternalRef `json:",omitempty"`
	Locations          Locations     `json:",omitempty"`
	FilePath           string        `json:",omitempty"` // Required to show nested jars
}

type Libraries []Library

func (libs Libraries) Len() int { return len(libs) }
func (libs Libraries) Less(i, j int) bool {
	if libs[i].ID != libs[j].ID { // ID could be empty
		return libs[i].ID < libs[j].ID
	} else if libs[i].Name != libs[j].Name { // Name could be the same
		return libs[i].Name < libs[j].Name
	}
	return libs[i].Version < libs[j].Version
}
func (libs Libraries) Swap(i, j int) { libs[i], libs[j] = libs[j], libs[i] }

// Location in lock file
type Location struct {
	StartLine int `json:",omitempty"`
	EndLine   int `json:",omitempty"`
}

type Locations []Location

func (locs Locations) Len() int { return len(locs) }
func (locs Locations) Less(i, j int) bool {
	return locs[i].StartLine < locs[j].StartLine
}
func (locs Locations) Swap(i, j int) { locs[i], locs[j] = locs[j], locs[i] }

type ExternalRef struct {
	Type RefType
	URL  string
}

type Requirement struct {
	Target, Constraint, Scope, Condition string
	Resolved                             string
}

type Dependency struct {
	Requirements []Requirement

	ID        string
	DependsOn []string
}

type Dependencies []Dependency

func (deps Dependencies) Len() int { return len(deps) }
func (deps Dependencies) Less(i, j int) bool {
	return deps[i].ID < deps[j].ID
}
func (deps Dependencies) Swap(i, j int) { deps[i], deps[j] = deps[j], deps[i] }

type Parser interface {
	// Parse parses the dependency file
	Parse(fs fi.FileSystem, r ReadSeekerAt) ([]Library, []Dependency, error)
}

type RefType string

const (
	RefWebsite      RefType = "website"
	RefLicense      RefType = "license"
	RefVCS          RefType = "vcs"
	RefIssueTracker RefType = "issue-tracker"
	RefOther        RefType = "other"
)

func ContextOf(r any) context.Context {
	if c, ok := r.(interface{ Context() context.Context }); ok {
		return c.Context()
	}
	return context.Background()
}
