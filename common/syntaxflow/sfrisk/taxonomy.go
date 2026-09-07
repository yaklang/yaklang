// Package sfrisk owns canonical built-in risk types and legacy display aliases.
// It never implicitly rewrites persisted findings or custom rule values.
package sfrisk

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const SchemaVersion = "yaklang_risk_type_taxonomy.v1"

//go:embed taxonomy.json
var taxonomyJSON []byte

type Taxonomy struct {
	SchemaVersion string     `json:"schema_version"`
	Version       string     `json:"version"`
	Categories    []Category `json:"categories"`
}

type Category struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Order       int    `json:"order"`
	RiskTypes   []Type `json:"risk_types"`
}

// Name is the canonical key required for new built-in risk metadata.
// Aliases are explicit, reviewed legacy spellings; no substring/CWE inference is used.
type Type struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"display_name"`
	Aliases        []string `json:"aliases,omitempty"`
	Order          int      `json:"order"`
	ReviewRequired bool     `json:"review_required,omitempty"`
}

type Definition struct {
	CanonicalName  string
	DisplayName    string
	CategoryID     string
	CategoryName   string
	CategoryOrder  int
	TypeOrder      int
	ReviewRequired bool
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

var builtins, definitions = mustLoadTaxonomy()

func mustLoadTaxonomy() (Taxonomy, map[string]Definition) {
	taxonomy, err := ParseTaxonomy(taxonomyJSON)
	if err != nil {
		panic(err)
	}
	index := make(map[string]Definition)
	for _, category := range taxonomy.Categories {
		for _, riskType := range category.RiskTypes {
			definition := Definition{
				CanonicalName: riskType.Name, DisplayName: riskType.DisplayName,
				CategoryID: category.ID, CategoryName: category.DisplayName,
				CategoryOrder: category.Order, TypeOrder: riskType.Order,
				ReviewRequired: riskType.ReviewRequired,
			}
			index[riskType.Name] = definition
			for _, alias := range riskType.Aliases {
				index[alias] = definition
			}
		}
	}
	return taxonomy, index
}

// GetTaxonomy returns a detached copy suitable for archive metadata.
func GetTaxonomy() Taxonomy {
	result := builtins
	result.Categories = append([]Category(nil), builtins.Categories...)
	for i := range result.Categories {
		result.Categories[i].RiskTypes = append([]Type(nil), builtins.Categories[i].RiskTypes...)
		for j := range result.Categories[i].RiskTypes {
			result.Categories[i].RiskTypes[j].Aliases = append([]string(nil), builtins.Categories[i].RiskTypes[j].Aliases...)
		}
	}
	return result
}

// Lookup resolves a reviewed spelling for display only. Unknown/custom values
// remain unknown; callers must preserve their raw value rather than guess a type.
func Lookup(raw string) (Definition, bool) {
	definition, ok := definitions[strings.TrimSpace(raw)]
	return definition, ok
}

// IsCanonical accepts only an active authoring key, not a legacy alias or review
// placeholder. Keep Lookup permissive for historical/custom rule consumers.
func IsCanonical(raw string) bool {
	definition, ok := definitions[raw]
	return ok && definition.CanonicalName == raw && !definition.ReviewRequired
}

func ParseTaxonomy(data []byte) (Taxonomy, error) {
	var taxonomy Taxonomy
	if err := json.Unmarshal(data, &taxonomy); err != nil {
		return Taxonomy{}, fmt.Errorf("decode risk type taxonomy: %w", err)
	}
	if taxonomy.SchemaVersion != SchemaVersion || strings.TrimSpace(taxonomy.Version) == "" || len(taxonomy.Categories) == 0 {
		return Taxonomy{}, fmt.Errorf("invalid risk type taxonomy version or empty categories")
	}
	categories := make(map[string]bool)
	names := make(map[string]string)
	for _, category := range taxonomy.Categories {
		if !identifier.MatchString(category.ID) || strings.TrimSpace(category.DisplayName) == "" || category.Order < 0 || len(category.RiskTypes) == 0 || categories[category.ID] {
			return Taxonomy{}, fmt.Errorf("invalid or duplicate risk type category %q", category.ID)
		}
		categories[category.ID] = true
		for _, riskType := range category.RiskTypes {
			if !identifier.MatchString(riskType.Name) || strings.TrimSpace(riskType.DisplayName) == "" || riskType.Order < 0 {
				return Taxonomy{}, fmt.Errorf("invalid risk type %q", riskType.Name)
			}
			for _, name := range append([]string{riskType.Name}, riskType.Aliases...) {
				if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
					return Taxonomy{}, fmt.Errorf("invalid risk type alias %q", name)
				}
				if previous, exists := names[name]; exists {
					return Taxonomy{}, fmt.Errorf("duplicate risk type name/alias %q in %q and %q", name, previous, riskType.Name)
				}
				names[name] = riskType.Name
			}
		}
	}
	return taxonomy, nil
}
