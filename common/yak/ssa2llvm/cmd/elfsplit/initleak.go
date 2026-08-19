package main

// Rejecting splits whose start-up path runs into prunable code.
//
// Package init tasks run unconditionally. If a function that stays in the base
// .text can reach — directly or through other base functions — code that moved
// into a .modtext group, then every binary that prunes that group executes a
// call into its stub during init. The stub panics with the group's name, so the
// script fails before its first statement.
//
// The fix is always the same: the offending package belongs in the group it
// calls into (or the callee belongs in the base), so this check exists to name
// the package at build time instead of leaving it for a user's first run.
//
// The graph is built from both kinds of reference elfsplit sees: relocations
// already present in .rela.text, and direct PC-relative branches, which carry
// no relocation and are only visible while decoding instructions. An earlier
// version looked at relocations alone and missed exactly the cases that matter
// — a plain `call` from an init function to a moved function.

import (
	"fmt"
	"sort"
	"strings"
)

// maxLeakPathHops bounds how much of a call path is printed. The first and last
// few frames identify the problem; the middle is noise.
const maxLeakPathHops = 6

// callGraph holds base-.text call edges plus, per function, the module groups
// it references. Only base functions are nodes: a call that already starts
// inside a group is that group's own business.
type callGraph struct {
	calls [][]int32
}

func newCallGraph(n int) *callGraph {
	return &callGraph{calls: make([][]int32, n)}
}

// addReference records that placements[src] references placements[dst].
func (g *callGraph) addReference(placements []codePlacement, src, dst int) {
	if src < 0 || dst < 0 || src == dst {
		return
	}
	g.calls[src] = append(g.calls[src], int32(dst))
}

// keptImplies reports whether keeping group a guarantees group b is kept.
//
// Each group is kept when the script uses one of the modules it was derived
// from, so the guarantee is exactly "a's module set is contained in b's". The
// base .text ("") is always kept and therefore implies nothing; a module's own
// group stands for that one module.
func keptImplies(a, b string) bool {
	if a == b || b == "" {
		return true
	}
	if a == "" {
		return false
	}
	// The shared core is kept as soon as any module is used, so every group
	// implies it and it implies no other group.
	if b == sharedGroup {
		return true
	}
	if a == sharedGroup {
		return false
	}
	for _, m := range groupModules(a) {
		if !moduleKeeps(m, b) {
			return false
		}
	}
	return true
}

// elfsplitModuleGroupDeps mirrors compiler/moduleGroupDeps: using a module
// keeps the groups it lists. The compiler's copy is authoritative; keep the
// two in sync so the start-up reachability check sees the same closure the
// linker will use.
var elfsplitModuleGroupDeps = map[string][]string{
	"poc":       {"cli", "sharednet"},
	"http":      {"cli", "poc", "sharednet"},
	"cli":       {"sharednet"},
	"ssa":       {"cli", "poc", "ssafront", "sharednet", "ai"},
	"ai":        {"ssa"},
	"liteforge": {"ai"},
	"sandbox":   {"ai"},
	"rag":       {"ai"},
	"dyn":       {"ai"},
	"hook":      {"ai"},
	"simulator": {"ssa"},
	"suricata":  {"ssa"},
	"pprof":     {"ssa"},
	"nuclei":    {"httptpl"},
	"atoi":      {"ssafront"},
	"bot":       {"ssafront"},
	"bufio":     {"ssafront"},
	"context":   {"ssafront"},
	"csrf":      {"ssafront"},
	"db":        {"ssafront"},
	"dictutil":  {"ssafront"},
	"dns":       {"ssafront"},
	"dnslog":    {"ssafront"},
	"env":       {"ssafront"},
	"exec":      {"ssafront"},
	"filemonitor": {"ssafront"},
	"filescanner": {"ssafront"},
	"fuzz":      {"ssafront"},
	"fuzzx":     {"ssafront"},
	"gzip":      {"ssafront"},
	"httpool":   {"ssafront"},
	"httpserver": {"ssafront"},
	"io":        {"ssafront"},
	"js":        {"ssafront"},
	"jsonstream": {"ssafront"},
	"ldap":      {"ssafront"},
	"math":      {"ssafront"},
	"mitm":      {"ssafront"},
	"mmdb":      {"ssafront"},
	"rdp":       {"ssafront"},
	"re":        {"ssafront"},
	"re2":       {"ssafront"},
	"redis":     {"ssafront"},
	"regen":     {"ssafront"},
	"risk":      {"ssafront"},
	"smb":       {"ssafront"},
	"spacengine": {"ssafront"},
	"ssh":       {"ssafront"},
	"tcp":       {"ssafront"},
	"timezone":  {"ssafront"},
	"tls":       {"ssafront"},
	"traceroute": {"ssafront"},
	"udp":       {"ssafront"},
	"x":         {"ssafront"},
	"xml":       {"ssafront"},
	"yaml":      {"ssafront"},
	"zip":       {"ssafront"},
	"brute":     {"tools"},
	"finscan":   {"tools"},
	"ping":      {"tools"},
	"servicescan": {"tools"},
	"subdomain": {"tools"},
	"synscan":   {"tools"},
}

// moduleKeeps reports whether using module m keeps group b, following the
// module dependency closure. A group is kept when the script uses one of its
// own modules or any module that transitively drags it in.
func moduleKeeps(m, b string) bool {
	return moduleKeepsSeen(m, b, map[string]bool{})
}

func moduleKeepsSeen(m, b string, seen map[string]bool) bool {
	if m == b {
		return true
	}
	if seen[m] {
		return false
	}
	seen[m] = true
	for _, need := range elfsplitModuleGroupDeps[m] {
		if moduleKeepsSeen(need, b, seen) {
			return true
		}
	}
	return false
}

// groupModules is the module set a group is kept for.
func groupModules(group string) []string {
	if mods, ok := generatedSharedGroupModules[group]; ok {
		return mods
	}
	if group == "ssafront" {
		return []string{"ssa"}
	}
	return []string{group}
}

type initLeak struct {
	group string
	path  []string
}

// initLeaks walks forward from every package init function and reports the
// paths that reach code the caller's own build may not have.
//
// The walk is done once per context — the group holding the init functions it
// starts from — because that group is what decides whether the path runs at
// all. An init in the ssa group only runs in a build that kept ssa, so its
// calls into the shared core are fine; the same call from an init in the base
// .text is not, because the base runs even when nothing is kept.
func (g *callGraph) initLeaks(placements []codePlacement) []initLeak {
	contexts := map[string]bool{}
	for i := range placements {
		if isInitFuncName(placements[i].name) {
			contexts[placements[i].module] = true
		}
	}
	reported := map[string]bool{}
	var leaks []initLeak
	for _, context := range sortedSet(contexts) {
		leaks = append(leaks, g.leaksFrom(placements, context, reported)...)
	}
	sort.Slice(leaks, func(i, j int) bool {
		if leaks[i].group != leaks[j].group {
			return leaks[i].group < leaks[j].group
		}
		return leaks[i].path[0] < leaks[j].path[0]
	})
	return leaks
}

// leaksFrom walks the init functions of one group. A call is followed when the
// callee is in code the context guarantees is present, and reported otherwise.
func (g *callGraph) leaksFrom(placements []codePlacement, context string, reported map[string]bool) []initLeak {
	parent := make([]int32, len(placements))
	for i := range parent {
		parent[i] = -1
	}
	seen := make([]bool, len(placements))
	var queue []int32
	for i := range placements {
		if placements[i].module == context && isInitFuncName(placements[i].name) {
			seen[i] = true
			queue = append(queue, int32(i))
		}
	}
	var leaks []initLeak
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		for _, next := range g.calls[node] {
			group := placements[next].module
			if !keptImplies(context, group) {
				root := placements[rootOf(parent, node)].name
				key := root + "\x00" + group
				if reported[key] {
					continue
				}
				reported[key] = true
				leaks = append(leaks, initLeak{
					group: group,
					path:  append(pathTo(parent, placements, node), placements[next].name),
				})
				continue
			}
			if !seen[next] {
				seen[next] = true
				parent[next] = node
				queue = append(queue, next)
			}
		}
	}
	return leaks
}

func rootOf(parent []int32, node int32) int32 {
	for parent[node] >= 0 {
		node = parent[node]
	}
	return node
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pathTo renders the chain from the init function down to node, eliding the
// middle when it is long.
func pathTo(parent []int32, placements []codePlacement, node int32) []string {
	var reversed []string
	for n := node; ; n = parent[n] {
		reversed = append(reversed, placements[n].name)
		if parent[n] < 0 {
			break
		}
	}
	path := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		path = append(path, reversed[i])
	}
	if len(path) > maxLeakPathHops {
		elided := append([]string{}, path[:maxLeakPathHops-1]...)
		elided = append(elided, fmt.Sprintf("... (%d more) ...", len(path)-maxLeakPathHops), path[len(path)-1])
		path = elided
	}
	return path
}

// formatInitLeaks turns leaks into the build error the developer reads.
func formatInitLeaks(leaks []initLeak) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d start-up path(s) reach code that would be pruned:\n", len(leaks))
	for _, leak := range leaks {
		fmt.Fprintf(&b, "  %s -> .modtext.%s\n", strings.Join(leak.path, " -> "), leak.group)
	}
	b.WriteString("these run on every start, so the pruned group's stub would panic.\n")
	b.WriteString("in buildModulePackageMap, move the calling package into that group, " +
		"or move the callee's package out of it")
	return fmt.Errorf("%s", b.String())
}

// isInitFuncName reports whether a symbol runs as part of package
// initialization: "<pkg>.init" (the generated task body), the "<pkg>.init.<n>"
// bodies of individual `func init()` blocks, and generated variants such as
// protobuf's "file_<name>_proto_init".
//
// Closures written inside an init ("<pkg>.init.func<n>") are deliberately not
// roots. Go names them after their enclosing function, but defining a closure
// is not running it — most are registration callbacks stored for later. They
// are still reached by this check whenever an init actually calls them.
func isInitFuncName(name string) bool {
	if strings.HasSuffix(name, "_init") {
		return true
	}
	idx := strings.LastIndex(name, ".init")
	if idx < 0 {
		return false
	}
	// A method named init ("pkg.(*T).init") is not a package init task; only
	// package-level init functions run at start-up.
	if strings.Contains(name[:idx], ".(") {
		return false
	}
	rest := name[idx+len(".init"):]
	if rest == "" {
		return true
	}
	if !strings.HasPrefix(rest, ".") {
		return false
	}
	return isAllDigits(rest[1:])
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
