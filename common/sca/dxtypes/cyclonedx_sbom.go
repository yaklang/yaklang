package dxtypes

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// BOM is the SCA-owned subset of CycloneDX 1.5. Relationships are represented
// by dependencies; components are not nested to simulate a dependency graph.
type BOM struct {
	Properties   []BOMProperty   `json:"properties,omitempty"`
	Schema       string          `json:"$schema"`
	BOMFormat    string          `json:"bomFormat"`
	SpecVersion  string          `json:"specVersion"`
	Version      int             `json:"version"`
	Components   []BOMComponent  `json:"components"`
	Dependencies []BOMDependency `json:"dependencies,omitempty"`
}
type BOMComponent struct {
	Type       string             `json:"type"`
	BOMRef     string             `json:"bom-ref"`
	Name       string             `json:"name"`
	Version    string             `json:"version,omitempty"`
	Hashes     []BOMHash          `json:"hashes,omitempty"`
	Licenses   []BOMLicenseChoice `json:"licenses,omitempty"`
	Properties []BOMProperty      `json:"properties,omitempty"`
	CPE        string             `json:"cpe,omitempty"`
}
type BOMProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type BOMHash struct {
	Algorithm string `json:"alg"`
	Value     string `json:"content"`
}
type BOMLicense struct {
	Name string `json:"name"`
}
type BOMLicenseChoice struct {
	License    *BOMLicense `json:"license,omitempty"`
	Expression string      `json:"expression,omitempty"`
}
type BOMDependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn"`
}

func normalCyloneDXHashType(s string) (string, bool) {
	k := strings.ToLower(strings.ReplaceAll(s, "_", "-"))
	switch k {
	case "md5":
		return "MD5", true
	case "sha1", "sha-1":
		return "SHA-1", true
	case "sha256", "sha-256":
		return "SHA-256", true
	case "sha384", "sha-384":
		return "SHA-384", true
	case "sha512", "sha-512":
		return "SHA-512", true
	case "sha3-256", "sha3-384", "sha3-512":
		return strings.ToUpper(k), true
	case "blake2b-256", "blake2b-384", "blake2b-512":
		return "BLAKE2b-" + strings.TrimPrefix(k, "blake2b-"), true
	case "blake3":
		return "BLAKE3", true
	}
	return "", false
}

func CreateCycloneDXSBOMByDXPackages(pkgs []*Package) *BOM {
	bom := &BOM{Schema: "http://cyclonedx.org/schema/bom-1.5.schema.json", BOMFormat: "CycloneDX", SpecVersion: "1.5", Version: 1, Components: []BOMComponent{}}
	// Track pointers for traversal and exact references for output. Pointer tracking
	// visits every occurrence's edges, even when two records share a reference.
	seen := map[*Package]bool{}
	components := map[string]BOMComponent{}
	edges := map[string]map[string]bool{}
	queue := append([]*Package(nil), pkgs...)
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if p == nil || seen[p] {
			continue
		}
		seen[p] = true
		id := p.Identifier()
		c, exists := components[id]
		if !exists {
			c = BOMComponent{Type: "library", BOMRef: id, Name: p.Name, Version: p.Version}
		}
		licenseEvidence := p.Details().RawLicenses
		if len(licenseEvidence) == 0 {
			licenseEvidence = p.License
		}
		for _, l := range licenseEvidence {
			choice := BOMLicenseChoice{License: &BOMLicense{Name: l}}
			if strings.Contains(l, " AND ") || strings.Contains(l, " OR ") || strings.Contains(l, " WITH ") {
				choice = BOMLicenseChoice{Expression: l}
			}
			duplicate := false
			raw, _ := json.Marshal(choice)
			for _, old := range c.Licenses {
				r, _ := json.Marshal(old)
				if string(r) == string(raw) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				c.Licenses = append(c.Licenses, choice)
			}
		}
		for _, part := range strings.Fields(p.Verification) {
			if alg, value, ok := strings.Cut(part, ":"); ok {
				if a, ok := normalCyloneDXHashType(alg); ok && validHash(a, value) {
					h := BOMHash{a, value}
					exists := false
					for _, old := range c.Hashes {
						if old == h {
							exists = true
							break
						}
					}
					if !exists {
						c.Hashes = append(c.Hashes, h)
					}
				}
			}
		}
		if p.Verification != "" {
			prop := BOMProperty{"sca:verification", p.Verification}
			found := false
			for _, old := range c.Properties {
				if old == prop {
					found = true
				}
			}
			if !found {
				c.Properties = append(c.Properties, prop)
			}
		}
		if details := p.Details(); details.DeclaredIntegrity != "" {
			prop := BOMProperty{"sca:declared-integrity", details.DeclaredIntegrity}
			found := false
			for _, old := range c.Properties {
				if old == prop {
					found = true
				}
			}
			if !found {
				c.Properties = append(c.Properties, prop)
			}
		}
		for _, cpe := range p.AmendedCPE {
			if c.CPE == "" || cpe < c.CPE {
				c.CPE = cpe
			}
		}
		components[id] = c
		if edges[id] == nil {
			edges[id] = map[string]bool{}
		}
		for _, up := range p.UpStreamPackages {
			if up != nil {
				edges[id][up.Identifier()] = true
				queue = append(queue, up)
			}
		}
		for _, down := range p.DownStreamPackages {
			if down != nil {
				if edges[down.Identifier()] == nil {
					edges[down.Identifier()] = map[string]bool{}
				}
				edges[down.Identifier()][id] = true
				queue = append(queue, down)
			}
		}
	}
	for id, c := range components {
		// CycloneDX permits a single expression OR a list of license objects.
		// Multiple observations are evidence, not an invented AND expression.
		if len(c.Licenses) > 1 {
			for i := range c.Licenses {
				if c.Licenses[i].Expression != "" {
					c.Licenses[i] = BOMLicenseChoice{License: &BOMLicense{Name: c.Licenses[i].Expression}}
				}
			}
		}

		sort.Slice(c.Hashes, func(i, j int) bool {
			if c.Hashes[i].Algorithm != c.Hashes[j].Algorithm {
				return c.Hashes[i].Algorithm < c.Hashes[j].Algorithm
			}
			return c.Hashes[i].Value < c.Hashes[j].Value
		})
		sort.Slice(c.Licenses, func(i, j int) bool {
			a, _ := json.Marshal(c.Licenses[i])
			b, _ := json.Marshal(c.Licenses[j])
			return string(a) < string(b)
		})
		bom.Components = append(bom.Components, c)
		refs := []string{}
		for ref := range edges[id] {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		bom.Dependencies = append(bom.Dependencies, BOMDependency{id, refs})
	}
	sort.Slice(bom.Components, func(i, j int) bool { return bom.Components[i].BOMRef < bom.Components[j].BOMRef })
	sort.Slice(bom.Dependencies, func(i, j int) bool { return bom.Dependencies[i].Ref < bom.Dependencies[j].Ref })
	return bom
}
func MarshalCycloneDXBomToJSON(bom *BOM) ([]byte, error) {
	if bom == nil {
		return nil, fmt.Errorf("nil CycloneDX BOM")
	}
	return json.Marshal(bom)
}

func validHash(algorithm, value string) bool {
	sizes := map[string]int{"MD5": 32, "SHA-1": 40, "SHA-256": 64, "SHA-384": 96, "SHA-512": 128, "SHA3-256": 64, "SHA3-384": 96, "SHA3-512": 128, "BLAKE2b-256": 64, "BLAKE2b-384": 96, "BLAKE2b-512": 128, "BLAKE3": 64}
	if len(value) != sizes[algorithm] {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
