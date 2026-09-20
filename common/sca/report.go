package sca

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
	"sort"
	"strings"
)

func fillReport(r *model.Report, pkgs []*dxtypes.Package, l ResourceLimits) {
	sort.SliceStable(pkgs, func(i, j int) bool {
		return pkgs[i].Identifier() < pkgs[j].Identifier()
	})
	observations := map[*dxtypes.Package]string{}
	actual := map[*dxtypes.Package]bool{}
	native := map[[3]string]string{}
	componentIDs := map[model.ComponentKey]bool{}
	providers := map[[4]string][]string{}
	providerCount := 0
	limit := func(what string) {
		r.Complete = false
		r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "resource_limit", Stage: "normalize", Reason: what, Incomplete: true})
	}
	for _, p := range pkgs {
		p.EnsureDetails()
		for _, d := range p.Diagnostics {
			if d.File == "" && len(p.FromFile) > 0 {
				d.File = p.FromFile[0]
			}
			r.Diagnostics = append(r.Diagnostics, d)
		}
		if p.Potential {
			continue
		}
		if len(r.Observations) >= l.MaxObservations {
			limit("component or observation count")
			break
		}
		version := p.Version
		if p.HasVersionRange() {
			version = ""
		}
		key := model.ComponentKey{Ecosystem: p.Ecosystem, Name: p.Name, Version: version, Source: p.Source, Architecture: p.Architecture, Variant: p.Variant, Verification: p.Verification}
		if !componentIDs[key] && len(componentIDs) >= l.MaxComponents {
			limit("component count")
			break
		}
		componentIDs[key] = true
		licenses := p.RawLicenses
		if len(licenses) == 0 {
			licenses = p.License
		}
		r.Components = append(r.Components, model.Component{Key: key, Licenses: append([]string(nil), licenses...)})
		kind := p.Evidence
		if kind == "" {
			kind = "locked"
		}
		file := ""
		if len(p.FromFile) > 0 {
			file = p.FromFile[0]
		}
		provides := append([]string(nil), p.Provides...)
		sort.Strings(provides)
		o := model.Observation{Provides: provides, Condition: p.Condition, Scope: p.Scope, Component: key.ID(), Snapshot: p.Snapshot, Project: p.ProjectRoot, File: file, NativeID: p.Instance, Kind: kind, StartLine: p.StartLine, EndLine: p.EndLine}
		r.Observations = append(r.Observations, o)
		observations[p] = o.ID()
		nativeKey := [3]string{p.Snapshot, p.ProjectRoot, p.Instance}
		if prev, exists := native[nativeKey]; exists && prev != o.ID() {
			native[nativeKey] = ""
			r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "evidence_insufficient", Stage: "resolve", File: file, Reason: "ambiguous native record: " + p.Instance, Incomplete: true})
		} else {
			native[nativeKey] = o.ID()
		}
		for _, loc := range p.Locations {
			if loc.StartLine == o.StartLine && loc.EndLine == o.EndLine {
				continue
			}
			if len(r.Observations) >= l.MaxObservations {
				limit("observation count")
				break
			}
			extra := o
			extra.StartLine, extra.EndLine = loc.StartLine, loc.EndLine
			r.Observations = append(r.Observations, extra)
		}
		actual[p] = true
		if p.Evidence == "installed" {
			for _, raw := range append([]string{p.Name}, p.Provides...) {
				name := raw
				if p.Ecosystem == "apk" {
					if at := strings.IndexAny(raw, "<>="); at >= 0 {
						name = raw[:at]
					}
				}
				if p.Ecosystem == "dpkg" {
					if n, _, ok := strings.Cut(raw, " ("); ok {
						name = n
					}
				}
				if providerCount >= l.MaxEdges {
					limit("capability index count")
					break
				}
				providerCount++
				k := [4]string{p.Snapshot, p.ProjectRoot, p.Ecosystem, name}
				providers[k] = append(providers[k], o.ID())
			}
		}
	}
	edgeLimitReported := false
	candidateCount := 0
	add := func(q model.Requirement) {
		if len(r.Requirements) >= l.MaxEdges {
			if !edgeLimitReported {
				edgeLimitReported = true
				limit("requirement count")
			}
			return
		}
		r.Requirements = append(r.Requirements, q)
	}
	for _, p := range pkgs {
		from := observations[p]
		if !p.Potential && from == "" {
			continue
		}
		if p.Potential {
			if len(p.DownStreamPackages) == 0 {
				add(model.Requirement{Target: p.Name, Constraint: p.Version, Scope: "declaration"})
			}
			continue
		}
		// AND and OR groups are original declarations. Do not split composite IDs or
		// choose an alternative based on map order or an unrelated installed name.
		names := make([]string, 0, len(p.DependsOn.And))
		for n := range p.DependsOn.And {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			q := model.Requirement{From: from, Target: n, Constraint: p.DependsOn.And[n], Operator: "and"}
			for _, id := range providers[[4]string{p.Snapshot, p.ProjectRoot, p.Ecosystem, n}] {
				if candidateCount >= l.MaxEdges {
					if !edgeLimitReported {
						limit("provider candidate count")
						edgeLimitReported = true
					}
					break
				}
				q.Candidates = append(q.Candidates, id)
				candidateCount++
			}
			add(q)
		}
		for i, alternatives := range p.DependsOn.Or {
			names = names[:0]
			for n := range alternatives {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				add(model.Requirement{From: from, Target: n, Constraint: alternatives[n], Operator: "or", Group: fmt.Sprintf("%s:%d", from, i)})
			}
		}
		handled := map[string]bool{}
		for _, q := range p.Requirements {
			for _, ref := range q.Resolved {
				handled[ref] = true
			}
		}
		for _, up := range p.UpStreamPackages {
			if up == nil || handled[up.Instance] {
				continue
			}
			if up.Potential {
				if len(p.DependsOn.And) == 0 && len(p.DependsOn.Or) == 0 {
					add(model.Requirement{From: from, Target: up.Name, Constraint: up.Version})
				}
				continue
			}
			q := model.Requirement{From: from, Target: up.Instance}
			if actual[up] && p.ProjectRoot == up.ProjectRoot && p.Snapshot == up.Snapshot {
				q.Resolved = []string{observations[up]}
			}
			add(q)
		}
		for _, q := range p.Requirements {
			q.From = from
			refs := q.Resolved
			q.Resolved = nil
			for _, ref := range refs {
				if id := native[[3]string{p.Snapshot, p.ProjectRoot, ref}]; id != "" {
					q.Resolved = append(q.Resolved, id)
				}
			}
			add(q)
		}
		for _, ref := range p.UnresolvedDependencies {
			add(model.Requirement{From: from, Target: ref})
		}
		if p.DeclaredName != "" {
			q := model.Requirement{From: from, Target: p.DeclaredName, Constraint: p.DeclaredVersion, Condition: p.DeclaredCondition, Scope: "declaration"}
			if p.Indirect {
				q.Scope = "indirect"
			}
			add(q)
		} else if p.HasVersionRange() {
			add(model.Requirement{From: from, Target: p.Name, Constraint: p.Version, Scope: "declaration"})
		}
	}
	// Rebuild compatibility relationship map keys after assigning identities.
	edges := map[*dxtypes.Package][]*dxtypes.Package{}
	for _, p := range pkgs {
		for _, q := range p.UpStreamPackages {
			if q != nil {
				edges[p] = append(edges[p], q)
			}
		}
		p.UpStreamPackages = nil
		p.DownStreamPackages = nil
	}
	for p, ups := range edges {
		for _, up := range ups {
			p.LinkDepend(up)
		}
	}
}
func manifestEvidence(file string) bool {
	return strings.HasSuffix(file, "/package.json") || file == "package.json" || strings.HasSuffix(file, "/composer.json") || file == "composer.json" || strings.HasSuffix(file, "requirements.txt")
}
