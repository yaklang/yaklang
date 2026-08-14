// moduledeps answers the question a shared-group split depends on: for every
// package currently in .modtext.shared, which yaklib modules actually need it?
//
// It takes each module's AOT entry package from the module registry, asks the
// Go toolchain for that package's dependency closure, and groups the shared
// packages by the exact set of modules that reach them. Packages with the same
// set can share one section: a script keeps that section if and only if it uses
// at least one module in the set.
//
// Usage: moduledeps <module>=<import path> [...]
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: moduledeps <module>=<import path> [...]")
		os.Exit(2)
	}
	modulesByPackage := map[string]map[string]bool{}
	var moduleNames []string
	for _, arg := range os.Args[1:] {
		name, path, ok := strings.Cut(arg, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "bad argument %q, want <module>=<import path>\n", arg)
			os.Exit(2)
		}
		moduleNames = append(moduleNames, name)
		deps, err := listDeps(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			os.Exit(1)
		}
		for _, dep := range deps {
			if modulesByPackage[dep] == nil {
				modulesByPackage[dep] = map[string]bool{}
			}
			modulesByPackage[dep][name] = true
		}
		fmt.Fprintf(os.Stderr, "%-8s %s -> %d packages\n", name, path, len(deps))
	}
	sort.Strings(moduleNames)

	// Group packages by the module set that needs them.
	bySignature := map[string][]string{}
	for pkg, mods := range modulesByPackage {
		names := make([]string, 0, len(mods))
		for m := range mods {
			names = append(names, m)
		}
		sort.Strings(names)
		signature := strings.Join(names, "+")
		bySignature[signature] = append(bySignature[signature], pkg)
	}

	signatures := make([]string, 0, len(bySignature))
	for s := range bySignature {
		signatures = append(signatures, s)
	}
	sort.Slice(signatures, func(i, j int) bool {
		if len(bySignature[signatures[i]]) != len(bySignature[signatures[j]]) {
			return len(bySignature[signatures[i]]) > len(bySignature[signatures[j]])
		}
		return signatures[i] < signatures[j]
	})
	if out := os.Getenv("MODULEDEPS_GO"); out != "" {
		if err := writeGroups(out, bySignature); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// "-list" prints one "<module set>\t<package>" line per package, for
	// joining against section sizes.
	if os.Getenv("MODULEDEPS_LIST") != "" {
		for _, s := range signatures {
			pkgs := bySignature[s]
			sort.Strings(pkgs)
			for _, p := range pkgs {
				fmt.Printf("%s\t%s\n", s, p)
			}
		}
		return
	}
	fmt.Printf("%d distinct module sets over %d packages\n", len(signatures), len(modulesByPackage))
	for _, s := range signatures {
		pkgs := bySignature[s]
		sort.Strings(pkgs)
		fmt.Printf("\n=== %s (%d packages) ===\n", s, len(pkgs))
		for i, p := range pkgs {
			if i == 12 {
				fmt.Printf("  ... and %d more\n", len(pkgs)-12)
				break
			}
			fmt.Printf("  %s\n", p)
		}
	}
}

// splitGroups names the dependency classes that become their own section.
// A class is identified by the exact set of modules whose closure contains it,
// so keeping the section is decided by "does the script use any of these".
// Classes containing aotlib are universal (it backs codec/os/str) and stay in
// the shared core; the ssa-only class already has its own groups.
//
// Both prunable classes map to one group on purpose. Splitting them further
// would only pay off for a cli-only script, and it would introduce group pairs
// where one is kept and the other pruned — the init path of the kept one then
// has to avoid the pruned one, which is not true of these packages today.
//
// Every non-universal class must be mapped somewhere, and to a group kept no
// more often than the class demands. That is what keeps the assignment
// consistent: if p is only reached through module set S, everything p imports
// is reached through at least S, so it lands in a group that is kept whenever
// p's is. Leaving a class in the always-kept core breaks the invariant — the
// core would then call into a group a codec-only script prunes.
//
// The ssa-only class joins ssafront, which already has exactly the same keep
// condition (the script uses ssa), rather than adding a third section.
var splitGroups = map[string]string{
	"cli+http+poc+ssa": "sharednet",
	"http+poc+ssa":     "sharednet",
	"ssa":              "ssafront",
}

// loadFilter reads the packages a split is allowed to move. Only packages the
// group already contains may be listed: anything else is code the base runtime
// or another group keeps, and moving it into a prunable section makes a pruned
// build call a stub.
func loadFilter(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			allowed[line] = true
		}
	}
	return allowed, nil
}

// coreClosure returns the packages that must stay in the shared core.
//
// The group also holds packages no module's closure reaches — they are linked
// because something in the base runtime imports them, and their init tasks run
// whenever the core is kept. Whatever they depend on therefore has to be kept
// with them: moving it into a subgroup that a codec-only script prunes turns
// their init into a call to that subgroup's stub.
func coreClosure(allowed map[string]bool, bySignature map[string][]string) (map[string]bool, error) {
	classified := map[string]bool{}
	for _, pkgs := range bySignature {
		for _, p := range pkgs {
			classified[p] = true
		}
	}
	var roots []string
	for p := range allowed {
		// Symbols of generic instantiations carry type arguments in their
		// name and do not parse as import paths; a plausible path has no
		// spaces or punctuation of that kind.
		if !classified[p] && isImportPath(p) {
			roots = append(roots, p)
		}
	}
	if len(roots) == 0 {
		return nil, nil
	}
	sort.Strings(roots)
	deps, err := listDeps(roots...)
	if err != nil {
		return nil, fmt.Errorf("core closure: %w", err)
	}
	pinned := make(map[string]bool, len(deps))
	for _, d := range deps {
		pinned[d] = true
	}
	fmt.Fprintf(os.Stderr, "core-only packages: %d, their closure pins %d packages\n", len(roots), len(pinned))
	return pinned, nil
}

func isImportPath(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t{}[]()*;")
}

func writeGroups(path string, bySignature map[string][]string) error {
	var b strings.Builder
	b.WriteString("// Code generated by common/yak/ssa2llvm/cmd/moduledeps. DO NOT EDIT.\n//\n")
	b.WriteString("// Each group holds the packages reached by exactly one set of yaklib\n")
	b.WriteString("// modules, as reported by `go list -deps` on those modules' AOT entry\n")
	b.WriteString("// packages. A script keeps the group when it uses any module in that set,\n")
	b.WriteString("// which is what compile_dispatch_selfcontained.go encodes.\n//\n")
	b.WriteString("// To regenerate, see the reproduction steps in\n")
	b.WriteString("// common/yak/ssa2llvm/docs/article-link-time-pruning.md (section 8).\n\n")
	var allowed map[string]bool
	var pinned map[string]bool
	if filter := os.Getenv("MODULEDEPS_FILTER"); filter != "" {
		var err error
		if allowed, err = loadFilter(filter); err != nil {
			return err
		}
		if pinned, err = coreClosure(allowed, bySignature); err != nil {
			return err
		}
		b.WriteString("// Only packages already in the shared group are listed; the set comes\n")
		b.WriteString("// from the group's own symbols, so nothing the base runtime keeps can\n")
		b.WriteString("// be moved into a prunable section.\n\n")
	}
	// Several dependency classes can share one group; the group is then kept
	// for the union of their module sets.
	pkgsByGroup := map[string][]string{}
	modulesByGroup := map[string]map[string]bool{}
	for _, signature := range sortedKeys(splitGroups) {
		group := splitGroups[signature]
		for _, p := range bySignature[signature] {
			if (allowed == nil || allowed[p]) && !pinned[p] {
				pkgsByGroup[group] = append(pkgsByGroup[group], p)
			}
		}
		if modulesByGroup[group] == nil {
			modulesByGroup[group] = map[string]bool{}
		}
		for _, m := range strings.Split(signature, "+") {
			modulesByGroup[group][m] = true
		}
	}

	b.WriteString("package main\n\n")
	b.WriteString("var generatedSharedGroups = map[string][]string{\n")
	for _, group := range sortedGroups(pkgsByGroup) {
		pkgs := pkgsByGroup[group]
		sort.Strings(pkgs)
		fmt.Fprintf(&b, "\t// kept when the script uses any of: %s\n", strings.Join(sortedSet(modulesByGroup[group]), ", "))
		fmt.Fprintf(&b, "\t%q: {\n", group)
		for _, p := range pkgs {
			fmt.Fprintf(&b, "\t\t%q,\n", p)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// generatedSharedGroupModules is the module set each group was derived\n")
	b.WriteString("// from: the group is kept exactly when the script uses one of them.\n")
	b.WriteString("var generatedSharedGroupModules = map[string][]string{\n")
	for _, group := range sortedGroups(pkgsByGroup) {
		fmt.Fprintf(&b, "\t%q: {", group)
		for i, m := range sortedSet(modulesByGroup[group]) {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", m)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func sortedGroups(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func listDeps(importPaths ...string) ([]string, error) {
	// -e keeps a package that cannot be loaded from failing the whole query.
	cmd := exec.Command("go", append([]string{"list", "-e", "-deps"}, importPaths...)...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var deps []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			deps = append(deps, line)
		}
	}
	return deps, nil
}
