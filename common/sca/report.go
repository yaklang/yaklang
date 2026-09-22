package sca

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
	"sort"
	"strings"
)

func locationKey(p *dxtypes.Package) string {
	p.EnsureDetails()
	locs := append([]dxtypes.SourceRange{{StartLine: p.StartLine, EndLine: p.EndLine}}, p.Locations...)
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].StartLine != locs[j].StartLine {
			return locs[i].StartLine < locs[j].StartLine
		}
		return locs[i].EndLine < locs[j].EndLine
	})
	var b strings.Builder
	for _, loc := range locs {
		fmt.Fprintf(&b, "%d:%d,", loc.StartLine, loc.EndLine)
	}
	return b.String()
}

func requirementKey(p *dxtypes.Package) string {
	p.EnsureDetails()
	reqs := append([]model.Requirement(nil), p.Requirements...)
	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].Target != reqs[j].Target {
			return reqs[i].Target < reqs[j].Target
		}
		if reqs[i].Constraint != reqs[j].Constraint {
			return reqs[i].Constraint < reqs[j].Constraint
		}
		return reqs[i].Scope < reqs[j].Scope
	})
	raw, _ := json.Marshal(reqs)
	return string(raw)
}

func locationWorkingBytes(p *dxtypes.Package) (int64, error) {
	n := 1 + len(p.Locations)
	pair, err := budget.SizeMul(n, 24)
	if err != nil {
		return 0, err
	}
	copyCh, err := budget.SizeMul(n, budget.SizeObject)
	if err != nil {
		return 0, err
	}
	return budget.SizeAdd(budget.SizeSlice, budget.SizeString, pair, copyCh)
}

func requirementWorkingBytes(p *dxtypes.Package) (int64, error) {
	n, err := budget.SizeAdd(budget.SizeSlice, budget.SizeString)
	if err != nil {
		return 0, err
	}
	for _, q := range p.Requirements {
		parts := []int64{budget.SizeObject}
		for _, s := range []string{q.Target, q.Constraint, q.Scope, q.From, q.Group, q.Operator, q.Condition} {
			js, e := budget.SizeOfJSONString(s)
			if e != nil {
				return 0, e
			}
			parts = append(parts, js)
		}
		for _, s := range q.Candidates {
			js, e := budget.SizeOfJSONString(s)
			if e != nil {
				return 0, e
			}
			parts = append(parts, js)
		}
		for _, s := range q.Resolved {
			js, e := budget.SizeOfJSONString(s)
			if e != nil {
				return 0, e
			}
			parts = append(parts, js)
		}
		obj, e := budget.SizeAdd(parts...)
		if e != nil {
			return 0, e
		}
		n, err = budget.SizeAdd(n, obj)
		if err != nil {
			return 0, err
		}
	}
	return n, nil
}

func chargeAndSortPackages(st *budget.State, pkgs []*dxtypes.Package) error {
	n := len(pkgs)
	if n == 0 {
		return nil
	}
	digestCh, err := budget.SizeMul(n, 32)
	if err != nil {
		return err
	}
	orderCh, err := budget.SizeMul(n, budget.SizePtr)
	if err != nil {
		return err
	}
	sortedCh, err := budget.SizeMul(n, budget.SizePtr)
	if err != nil {
		return err
	}
	total, err := budget.SizeAdd(digestCh, orderCh, sortedCh)
	if err != nil {
		return err
	}
	if err := st.Result(total); err != nil {
		return err
	}
	digests := make([][32]byte, n)
	for i, p := range pkgs {
		p.EnsureDetails()
		digests[i] = p.IdentityDigest()
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return bytes.Compare(digests[order[i]][:], digests[order[j]][:]) < 0
	})
	for lo := 0; lo < n; {
		hi := lo + 1
		for hi < n && digests[order[hi]] == digests[order[lo]] {
			hi++
		}
		if hi-lo > 1 {
			if err := sortCollisionGroup(st, pkgs, order[lo:hi]); err != nil {
				return err
			}
		}
		lo = hi
	}
	sorted := make([]*dxtypes.Package, n)
	for i, j := range order {
		sorted[i] = pkgs[j]
	}
	copy(pkgs, sorted)
	return nil
}

func sortCollisionGroup(st *budget.State, pkgs []*dxtypes.Package, group []int) error {
	g := len(group)
	headers, err := budget.SizeMul(g, budget.SizeString)
	if err != nil {
		return err
	}
	var locNeed int64
	for _, pi := range group {
		w, e := locationWorkingBytes(pkgs[pi])
		if e != nil {
			return e
		}
		locNeed, err = budget.SizeAdd(locNeed, w)
		if err != nil {
			return err
		}
	}
	need, err := budget.SizeAdd(headers, locNeed)
	if err != nil {
		return err
	}
	if err := st.Result(need); err != nil {
		return err
	}
	locs := make([]string, g)
	for i, pi := range group {
		locs[i] = locationKey(pkgs[pi])
	}
	perm, err := budget.SizeMul(g, budget.SizePtr)
	if err != nil {
		return err
	}
	perm2, err := budget.SizeMul(g, budget.SizePtr)
	if err != nil {
		return err
	}
	copyHeaders, err := budget.SizeMul(g, budget.SizeString)
	if err != nil {
		return err
	}
	extra, err := budget.SizeAdd(perm, perm2, copyHeaders)
	if err != nil {
		return err
	}
	if err := st.Result(extra); err != nil {
		return err
	}
	idx := make([]int, g)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return locs[idx[i]] < locs[idx[j]] })
	newGroup := make([]int, g)
	newLocs := make([]string, g)
	for i, p := range idx {
		newGroup[i] = group[p]
		newLocs[i] = locs[p]
	}
	copy(group, newGroup)
	locs = newLocs
	for lo := 0; lo < g; {
		hi := lo + 1
		for hi < g && locs[hi] == locs[lo] {
			hi++
		}
		if hi-lo > 1 {
			if err := sortRequirementGroup(st, pkgs, group[lo:hi]); err != nil {
				return err
			}
		}
		lo = hi
	}
	return nil
}

func sortRequirementGroup(st *budget.State, pkgs []*dxtypes.Package, group []int) error {
	g := len(group)
	headers, err := budget.SizeMul(g, budget.SizeString)
	if err != nil {
		return err
	}
	idxCh, err := budget.SizeMul(g, budget.SizePtr)
	if err != nil {
		return err
	}
	outCh, err := budget.SizeMul(g, budget.SizePtr)
	if err != nil {
		return err
	}
	var jsonNeed int64
	for _, pi := range group {
		w, e := requirementWorkingBytes(pkgs[pi])
		if e != nil {
			return e
		}
		jsonNeed, err = budget.SizeAdd(jsonNeed, w)
		if err != nil {
			return err
		}
	}
	need, err := budget.SizeAdd(headers, jsonNeed, idxCh, outCh)
	if err != nil {
		return err
	}
	if err := st.Result(need); err != nil {
		return err
	}
	reqs := make([]string, g)
	for i, pi := range group {
		reqs[i] = requirementKey(pkgs[pi])
	}
	idx := make([]int, g)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return reqs[idx[i]] < reqs[idx[j]] })
	out := make([]int, g)
	for i, p := range idx {
		out[i] = group[p]
	}
	copy(group, out)
	return nil
}

func fillReport(r *model.Report, pkgs []*dxtypes.Package, l ResourceLimits, shared ...*budget.State) []error {
	return fillReportMode(r, pkgs, l, true, shared...)
}

// A complete report is ordered by Normalize. Only compatibility output and
// potentially truncated graphs need the extra package identity sort first.
// Upper bounds deliberately count duplicates and unresolved legacy edges.
func reportCannotTruncate(pkgs []*dxtypes.Package, l ResourceLimits) bool {
	if len(pkgs) > l.MaxComponents {
		return false
	}
	observations, requirements, providers, declarations := 0, 0, 0, 0
	add := func(total *int, n, limit int) bool {
		if n < 0 || n > limit-*total {
			return false
		}
		*total += n
		return true
	}
	for _, p := range pkgs {
		p.EnsureDetails()
		if !add(&observations, max(1, len(p.Locations)), l.MaxObservations) {
			return false
		}
		if !add(&requirements, len(p.Requirements), l.MaxEdges) || !add(&requirements, len(p.DependsOn.And), l.MaxEdges) || !add(&requirements, len(p.UpStreamPackages), l.MaxEdges) || !add(&requirements, len(p.UnresolvedDependencies), l.MaxEdges) || !add(&requirements, 1, l.MaxEdges) {
			return false
		}
		for _, or := range p.DependsOn.Or {
			if !add(&requirements, len(or), l.MaxEdges) {
				return false
			}
		}
		if !add(&declarations, len(p.Requirements), l.MaxEdges) || !add(&declarations, len(p.DependsOn.And), l.MaxEdges) {
			return false
		}
		if p.Evidence == "installed" && !add(&providers, 1+len(p.Provides), l.MaxEdges) {
			return false
		}
	}
	return providers == 0 || declarations <= l.MaxEdges/providers
}

func fillReportMode(r *model.Report, pkgs []*dxtypes.Package, l ResourceLimits, compatibility bool, shared ...*budget.State) []error {
	st := &budget.State{Limits: l}
	if len(shared) > 0 && shared[0] != nil {
		st = shared[0]
	}
	index := len(pkgs)
	for _, p := range pkgs {
		p.EnsureDetails()
		index += len(p.Locations) + len(p.Requirements)
	}
	if err := st.Result(budget.SizeOfSortIndex(index)); err != nil {
		r.Complete = false
		r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "resource_limit", Stage: "normalize", Reason: "result memory estimate", Incomplete: true})
		return []error{scanerr.New(scanerr.ResourceLimit, "result memory estimate")}
	}
	sortPackages := compatibility || !reportCannotTruncate(pkgs, l)
	for _, p := range pkgs {
		p.EnsureDetails()
		sort.SliceStable(p.Locations, func(i, j int) bool {
			if p.Locations[i].StartLine != p.Locations[j].StartLine {
				return p.Locations[i].StartLine < p.Locations[j].StartLine
			}
			return p.Locations[i].EndLine < p.Locations[j].EndLine
		})
		if len(p.Locations) > 0 {
			p.StartLine, p.EndLine = p.Locations[0].StartLine, p.Locations[0].EndLine
		}
		if !sortPackages {
			continue
		}
		sort.SliceStable(p.Requirements, func(i, j int) bool {
			if p.Requirements[i].Target != p.Requirements[j].Target {
				return p.Requirements[i].Target < p.Requirements[j].Target
			}
			if p.Requirements[i].Constraint != p.Requirements[j].Constraint {
				return p.Requirements[i].Constraint < p.Requirements[j].Constraint
			}
			return p.Requirements[i].Scope < p.Requirements[j].Scope
		})
	}
	var sortErr error
	if sortPackages {
		sortErr = chargeAndSortPackages(st, pkgs)
	}
	if sortErr != nil {
		r.Complete = false
		r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "resource_limit", Stage: "normalize", Reason: "result memory estimate", Incomplete: true})
		return []error{scanerr.New(scanerr.ResourceLimit, "result memory estimate")}
	}
	observations := map[*dxtypes.Package]string{}
	actual := map[*dxtypes.Package]bool{}
	native := map[[3]string]string{}
	componentIDs := map[model.ComponentKey]bool{}
	providers := map[[4]string][]string{}
	providerCount := 0
	var errs []error
	resourceReported := false
	limit := func(what string) {
		if resourceReported {
			return
		}
		resourceReported = true
		r.Complete = false
		r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "resource_limit", Stage: "normalize", Reason: what, Incomplete: true})
		errs = append(errs, scanerr.New(scanerr.ResourceLimit, "%s", what))
	}
	for _, p := range pkgs {
		p.EnsureDetails()
		for _, d := range p.Diagnostics {
			if d.File == "" && len(p.FromFile) > 0 {
				d.File = p.FromFile[0]
			}
			if err := st.Result(budget.SizeObject + budget.SizeOfString(d.Reason)); err != nil {
				limit("result memory estimate")
				break
			}
			grown, err := budget.Grow(st, r.Diagnostics, 1, 128)
			if err != nil {
				limit("diagnostic memory estimate")
				return errs
			}
			r.Diagnostics = append(grown, d)
			if d.Incomplete {
				errs = append(errs, scanerr.Wrap(d.Code, fmt.Errorf("%s", d.Reason)))
			}
		}
		if st.Exhausted() {
			break
		}
		if p.Potential {
			continue
		}
		if strings.TrimSpace(p.Name) == "" {
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
		// Reserve indexes, JSON identity scratch and both backing arrays before
		// mutating the report. Field bytes can dominate the fixed DTO size.
		working := int64(512)
		for _, field := range []string{key.Ecosystem, key.Name, key.Version, key.Source, key.Architecture, key.Variant, key.Verification} {
			n, e := budget.SizeMul(len(field), 24)
			if e != nil {
				limit("component memory estimate")
				return errs
			}
			working, e = budget.SizeAdd(working, n)
			if e != nil {
				limit("component memory estimate")
				return errs
			}
		}
		if err := st.Working(working); err != nil {
			limit("component memory estimate")
			break
		}
		grown, err := budget.Grow(st, r.Components, 1, 160)
		if err != nil {
			limit("component memory estimate")
			break
		}
		r.Components = grown
		componentIDs[key] = true
		licenses := p.RawLicenses
		if len(licenses) == 0 {
			licenses = p.License
		}
		if err := st.Result(budget.SizeOfComponent(key.Name, key.Version)); err != nil {
			limit("result memory estimate")
			break
		}
		if err := st.Working(budget.SizeOfStrings(p.Provides) + budget.SizeOfStrings(licenses)); err != nil {
			limit("evidence memory estimate")
			break
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
		o := model.Observation{Provides: provides, Condition: p.Condition, Scope: p.Scope, Component: key.ID(), Snapshot: p.Snapshot, Project: p.ProjectRoot, File: file, NativeID: p.Instance, Kind: kind, StartLine: p.StartLine, EndLine: p.EndLine, DeclaredIntegrity: p.DeclaredIntegrity}
		if err := st.Result(budget.SizeOfObservation() + budget.SizeOfString(o.NativeID)); err != nil {
			limit("result memory estimate")
			break
		}
		working = 512
		for _, field := range []string{o.Condition, o.Scope, o.Component, o.Snapshot, o.Project, o.File, o.NativeID, o.Kind, o.DeclaredIntegrity} {
			n, e := budget.SizeMul(len(field), 24)
			if e != nil {
				limit("observation memory estimate")
				return errs
			}
			working, e = budget.SizeAdd(working, n)
			if e != nil {
				limit("observation memory estimate")
				return errs
			}
		}
		if err := st.Working(working); err != nil {
			limit("observation memory estimate")
			break
		}
		observationsOut, err := budget.Grow(st, r.Observations, 1, 256)
		if err != nil {
			limit("observation memory estimate")
			break
		}
		r.Observations = append(observationsOut, o)
		oid := o.ID()
		observations[p] = oid
		nativeKey := [3]string{p.Snapshot, p.ProjectRoot, p.Instance}
		if prev, exists := native[nativeKey]; exists && prev != oid {
			native[nativeKey] = ""
			if err := st.Working(256 + int64(len(p.Instance))); err != nil {
				limit("diagnostic memory estimate")
				return errs
			}
			grown, err := budget.Grow(st, r.Diagnostics, 1, 128)
			if err != nil {
				limit("diagnostic memory estimate")
				return errs
			}
			r.Diagnostics = append(grown, model.Diagnostic{Code: "evidence_insufficient", Stage: "resolve", File: file, Reason: "ambiguous native record: " + p.Instance, Incomplete: true})
		} else {
			native[nativeKey] = oid
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
			if err := st.Result(budget.SizeOfObservation()); err != nil {
				limit("result memory estimate")
				break
			}
			grown, err := budget.Grow(st, r.Observations, 1, 256)
			if err != nil {
				limit("observation memory estimate")
				break
			}
			r.Observations = append(grown, extra)
		}
		actual[p] = true
		if p.Evidence == "installed" {
			for i := 0; i <= len(p.Provides); i++ {
				raw := p.Name
				if i > 0 {
					raw = p.Provides[i-1]
				}
				name := raw
				if p.Ecosystem == "apk" || p.Ecosystem == "rpm" {
					if at := strings.IndexAny(raw, "<>="); at >= 0 {
						name = strings.TrimSpace(raw[:at])
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
				if _, exists := providers[k]; !exists {
					if err := st.Working(256); err != nil {
						limit("provider index memory estimate")
						return errs
					}
				}
				grown, err := budget.Grow(st, providers[k], 1, budget.SizeString)
				if err != nil {
					limit("provider index memory estimate")
					return errs
				}
				providers[k] = append(grown, oid)
			}
		}
	}
	if st.Exhausted() {
		return errs
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
		need, err := budget.SizeAdd(budget.SizeOfEdge(), budget.SizeOfString(q.Target), budget.SizeOfString(q.Constraint), budget.SizeOfString(q.Condition))
		if err != nil {
			if !edgeLimitReported {
				edgeLimitReported = true
				limit("result memory estimate")
			}
			return
		}
		if err := st.Result(need); err != nil {
			if !edgeLimitReported {
				edgeLimitReported = true
				limit("result memory estimate")
			}
			return
		}
		grown, err := budget.Grow(st, r.Requirements, 1, 256)
		if err != nil {
			limit("requirement memory estimate")
			return
		}
		r.Requirements = append(grown, q)
	}
	for _, p := range pkgs {
		if st.Exhausted() {
			return errs
		}
		from := observations[p]
		if !p.Potential && from == "" && strings.TrimSpace(p.Name) != "" {
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
		var names []string
		maxNames := len(p.DependsOn.And)
		for _, alternatives := range p.DependsOn.Or {
			if len(alternatives) > maxNames {
				maxNames = len(alternatives)
			}
		}
		names, err := budget.Grow(st, names, maxNames, budget.SizeString)
		if err != nil {
			limit("declaration index memory estimate")
			return errs
		}
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
				grown, err := budget.Grow(st, q.Candidates, 1, budget.SizeString)
				if err != nil {
					limit("candidate memory estimate")
					return errs
				}
				q.Candidates = append(grown, id)
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
			if err := st.Working(int64(len(from)) + 64); err != nil {
				limit("group memory estimate")
				return errs
			}
			group := fmt.Sprintf("%s:%d", from, i)
			for _, n := range names {
				add(model.Requirement{From: from, Target: n, Constraint: alternatives[n], Operator: "or", Group: group})
			}
		}
		handled := map[string]bool{}
		for _, q := range p.Requirements {
			for _, ref := range q.Resolved {
				if !handled[ref] {
					if err := st.Insert(ref); err != nil {
						limit("resolved index memory estimate")
						return errs
					}
					handled[ref] = true
				}
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
				if err := st.Working(budget.SizeSlice + budget.SizeString); err != nil {
					limit("resolved memory estimate")
					return errs
				}
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
					grown, err := budget.Grow(st, q.Resolved, 1, budget.SizeString)
					if err != nil {
						limit("resolved memory estimate")
						return errs
					}
					q.Resolved = append(grown, id)
				}
			}
			if len(q.Candidates) == 0 {
				for _, id := range providers[[4]string{p.Snapshot, p.ProjectRoot, p.Ecosystem, q.Target}] {
					if candidateCount >= l.MaxEdges {
						if !edgeLimitReported {
							limit("provider candidate count")
							edgeLimitReported = true
						}
						break
					}
					grown, err := budget.Grow(st, q.Candidates, 1, budget.SizeString)
					if err != nil {
						limit("candidate memory estimate")
						return errs
					}
					q.Candidates = append(grown, id)
					candidateCount++
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
	if !compatibility {
		return errs
	}
	// Rebuild compatibility relationship map keys after assigning identities.
	edges := map[*dxtypes.Package][]*dxtypes.Package{}
	for _, p := range pkgs {
		for _, q := range p.UpStreamPackages {
			if q != nil {
				if _, exists := edges[p]; !exists {
					if err := st.Working(128); err != nil {
						limit("relationship memory estimate")
						return errs
					}
				}
				grown, err := budget.Grow(st, edges[p], 1, budget.SizePtr)
				if err != nil {
					limit("relationship memory estimate")
					return errs
				}
				edges[p] = append(grown, q)
			}
		}
		p.UpStreamPackages = nil
		p.DownStreamPackages = nil
	}
	links := dxtypes.NewLinker(st)
	for p, ups := range edges {
		for _, up := range ups {
			if err := links.Link(p, up); err != nil {
				limit("relationship memory estimate")
				return errs
			}
		}
	}
	return errs
}
func manifestEvidence(file string) bool {
	return strings.HasSuffix(file, "/package.json") || file == "package.json" || strings.HasSuffix(file, "/composer.json") || file == "composer.json" || strings.HasSuffix(file, "requirements.txt")
}
