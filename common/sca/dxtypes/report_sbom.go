package dxtypes

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/sca/model"
	"sort"
	"strconv"
)

// CreateCycloneDXSBOMFromReport exports component identities separately from
// occurrence evidence. Only resolved requirements contribute dependency edges.
// Unknown requirements and scan completeness remain machine-readable properties.
func CreateCycloneDXSBOMFromReport(report *model.Report) *BOM {
	bom := CreateCycloneDXSBOMByDXPackages(nil)
	if report == nil {
		bom.Properties = []BOMProperty{{"sca:complete", "false"}}
		return bom
	}
	bom.Properties = []BOMProperty{{"sca:complete", strconv.FormatBool(report.Complete)}}
	add := func(name string, value any) {
		data, _ := json.Marshal(value)
		bom.Properties = append(bom.Properties, BOMProperty{name, string(data)})
	}
	add("sca:observations", report.Observations)
	add("sca:requirements", report.Requirements)
	add("sca:diagnostics", report.Diagnostics)
	components := map[string]BOMComponent{}
	for _, c := range report.Components {
		k := c.Key
		one := CreateCycloneDXSBOMByDXPackages([]*Package{{Name: k.Name, Version: k.Version, Verification: k.Verification, License: c.Licenses, PackageDetails: &PackageDetails{Ecosystem: k.Ecosystem, Source: k.Source, Architecture: k.Architecture, Variant: k.Variant}}}).Components[0]
		id := k.ID()
		one.BOMRef = id
		for _, p := range []BOMProperty{{"sca:ecosystem", k.Ecosystem}, {"sca:source", k.Source}, {"sca:architecture", k.Architecture}, {"sca:variant", k.Variant}} {
			if p.Value != "" {
				one.Properties = append(one.Properties, p)
			}
		}
		components[id] = one
	}
	occurrences := map[string]string{}
	for _, o := range report.Observations {
		occurrences[o.ID()] = o.Component
		if o.DeclaredIntegrity == "" {
			continue
		}
		c := components[o.Component]
		prop := BOMProperty{"sca:declared-integrity", o.DeclaredIntegrity}
		found := false
		for _, old := range c.Properties {
			if old == prop {
				found = true
			}
		}
		if !found {
			c.Properties = append(c.Properties, prop)
			components[o.Component] = c
		}
	}
	edges := map[string]map[string]bool{}
	for _, q := range report.Requirements {
		from := occurrences[q.From]
		if _, ok := components[from]; !ok {
			continue
		}
		if edges[from] == nil {
			edges[from] = map[string]bool{}
		}
		for _, ref := range q.Resolved {
			to := occurrences[ref]
			if _, ok := components[to]; ok {
				edges[from][to] = true
			}
		}
	}
	for id, c := range components {
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
