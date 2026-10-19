package java2ssa

import (
	"regexp"
	"sort"
	"strings"
	"text/scanner"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var (
	javaPackageRe = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_.]*)\s*;`)
	javaImportRe  = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][A-Za-z0-9_.]*)(?:\.\*)?\s*;`)
	javaStringRe  = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)
)

var javaTemplateExtensions = []string{".jsp", ".jspx", ".ftl", ".ftlh", ".ftlx", ".vm", ".vtl", ".html", ".htm"}

func (*SSABuilder) SupportsConcurrentUnitBuild() bool { return true }

func (*SSABuilder) DeferMethodBuildOnLookup() bool { return true }

// PartitionCompileUnits groups .java files by their declared package, and
// non-.java resources (templates, etc.) by directory under a "resource:" key.
func (*SSABuilder) PartitionCompileUnits(fs filesys_interface.FileSystem, files []string) []*ssa.CompileUnit {
	sort.Strings(files)
	units := make(map[string]*ssa.CompileUnit)
	order := make([]*ssa.CompileUnit, 0)
	add := func(key, unitPath, file string) {
		if key == "" {
			key = "unit:" + ssa.NormalizeUnitPath(fs, unitPath)
		}
		unit := units[key]
		if unit == nil {
			unit = &ssa.CompileUnit{Key: key, Path: unitPath, Language: ssaconfig.JAVA}
			units[key] = unit
			order = append(order, unit)
		}
		unit.Files = append(unit.Files, file)
		if info, err := fs.Stat(file); err == nil && info != nil {
			unit.Bytes += info.Size()
		}
	}
	for _, file := range files {
		unitPath := ssa.UnitDir(fs, file)
		key := "dir:" + ssa.NormalizeUnitPath(fs, unitPath)
		if strings.EqualFold(fs.Ext(file), ".java") {
			if pkg := scanJavaPackage(fs, file); pkg != "" {
				key = "java:" + pkg
				unitPath = ssa.PackagePath(pkg)
			}
		} else {
			key = "resource:" + ssa.NormalizeUnitPath(fs, unitPath)
		}
		add(key, unitPath, file)
	}
	for _, unit := range order {
		sort.Strings(unit.Files)
	}
	return order
}

// CompileUnitDependencies extracts Java import edges plus template-resource
// edges (template files referenced from .java string literals).
func (*SSABuilder) CompileUnitDependencies(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) []ssa.UnitRef {
	packageToKey := make(map[string]string)
	for _, unit := range units {
		if unit != nil && strings.HasPrefix(unit.Key, "java:") {
			packageToKey[strings.TrimPrefix(unit.Key, "java:")] = unit.Key
		}
	}
	templateFileToKey, templateBaseToKey := javaTemplateResourceIndexes(fs, units)
	var edges []ssa.UnitRef
	for _, unit := range units {
		for _, file := range unit.Files {
			if !strings.EqualFold(fs.Ext(file), ".java") {
				continue
			}
			src := ssa.ReadUnitSource(fs, file)
			for _, match := range javaImportRe.FindAllStringSubmatch(src, -1) {
				raw := match[1]
				if to := resolveJavaImport(packageToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "import", Raw: raw})
				}
			}
			// Imports are not the only dependency: fully qualified type and
			// static-member references need no import declaration. Treat these
			// as edges before scheduling packages concurrently.
			for _, raw := range javaQualifiedReferences(src) {
				if to := resolveJavaImport(packageToKey, raw); to != "" && to != unit.Key {
					edges = append(edges, ssa.UnitRef{From: unit.Key, To: to, Kind: "qualified", Raw: raw})
				}
			}
			for _, match := range javaStringRe.FindAllStringSubmatch(src, -1) {
				raw := javaUnquoteLight(match[1])
				if to := resolveJavaTemplateResource(fs, templateFileToKey, templateBaseToKey, raw); to != "" && to != unit.Key {
					edges = append(edges,
						ssa.UnitRef{From: unit.Key, To: to, Kind: "template", Raw: raw},
						ssa.UnitRef{From: to, To: unit.Key, Kind: "template-owner", Raw: raw},
					)
				}
			}
		}
	}
	return ssa.DedupeUnitRefs(edges)
}

func scanJavaPackage(fs filesys_interface.FileSystem, file string) string {
	src := ssa.ReadUnitSource(fs, file)
	match := javaPackageRe.FindStringSubmatch(src)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func resolveJavaImport(packageToKey map[string]string, raw string) string {
	for raw != "" {
		if key := packageToKey[raw]; key != "" {
			return key
		}
		end := strings.LastIndexByte(raw, '.')
		if end < 0 {
			break
		}
		raw = raw[:end]
	}
	return ""
}

// Scan identifiers rather than matching arbitrary source text, so comments,
// string literals and character literals cannot invent package dependencies.
// This is a conservative graph pass; the Java frontend remains the parser.
func javaQualifiedReferences(source string) []string {
	var lex scanner.Scanner
	lex.Init(strings.NewReader(source))
	lex.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanChars | scanner.ScanComments | scanner.SkipComments
	lex.Error = func(*scanner.Scanner, string) {} // Java escapes are validated by the frontend.
	var refs []string
	var name strings.Builder
	expectIdent := true
	qualified := false
	flush := func() {
		if qualified {
			refs = append(refs, strings.TrimSuffix(name.String(), "."))
		}
		name.Reset()
		expectIdent = true
		qualified = false
	}
	for tok := lex.Scan(); tok != scanner.EOF; tok = lex.Scan() {
		if tok == scanner.Ident {
			if !expectIdent {
				flush()
			}
			name.WriteString(lex.TokenText())
			expectIdent = false
		} else if tok == '.' && !expectIdent {
			name.WriteByte('.')
			expectIdent = true
			qualified = true
		} else {
			flush()
		}
	}
	flush()
	return refs
}

func javaTemplateResourceIndexes(fs filesys_interface.FileSystem, units []*ssa.CompileUnit) (map[string]string, map[string]string) {
	fileIndex := ssa.NewUniqueStringIndex()
	baseIndex := ssa.NewUniqueStringIndex()
	for _, unit := range units {
		for _, file := range unit.Files {
			if !isJavaTemplatePath(file) {
				continue
			}
			normalized := ssa.NormalizeUnitPath(fs, file)
			fileIndex.Add(normalized, unit.Key)
			if stem := stripJavaTemplateExtension(normalized); stem != normalized {
				fileIndex.Add(stem, unit.Key)
			}
			base := ssa.UnitBase(fs, normalized)
			baseIndex.Add(base, unit.Key)
			if stem := stripJavaTemplateExtension(base); stem != base {
				baseIndex.Add(stem, unit.Key)
			}
		}
	}
	return fileIndex.Values(), baseIndex.Values()
}

func resolveJavaTemplateResource(fs filesys_interface.FileSystem, fileToKey map[string]string, baseToKey map[string]string, raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return ""
	}
	raw = stripResourceSuffix(raw)
	trimmed := strings.TrimLeft(raw, "/")
	candidates := javaTemplateResourceCandidates(fs, trimmed)
	for _, candidate := range candidates {
		if key := fileToKey[ssa.NormalizeUnitPath(fs, candidate)]; key != "" {
			return key
		}
	}
	if key := baseToKey[ssa.UnitBase(fs, trimmed)]; key != "" {
		return key
	}
	return ""
}

func isJavaTemplatePath(path string) bool {
	return javaTemplateExtension(path) != ""
}

func javaTemplateResourceCandidates(fs filesys_interface.FileSystem, raw string) []string {
	raw = strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if raw == "" {
		return nil
	}
	prefixes := []string{
		raw,
		fs.Join("src/main/webapp", raw),
		fs.Join("src/main/resources/templates", raw),
		fs.Join("src/main/resources", raw),
		fs.Join("src/main/resource", raw),
	}
	seen := make(map[string]struct{})
	var candidates []string
	add := func(candidate string) {
		candidate = ssa.CleanUnitPath(fs, candidate)
		if candidate == "" || candidate == "." {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	for _, prefix := range prefixes {
		add(prefix)
	}
	if javaTemplateExtension(raw) == "" {
		for _, prefix := range prefixes {
			for _, ext := range javaTemplateExtensions {
				add(prefix + ext)
			}
		}
	}
	return candidates
}

func javaTemplateExtension(path string) string {
	path = strings.ToLower(stripResourceSuffix(path))
	for _, ext := range javaTemplateExtensions {
		if strings.HasSuffix(path, ext) {
			return ext
		}
	}
	return ""
}

func stripJavaTemplateExtension(path string) string {
	ext := javaTemplateExtension(path)
	if ext == "" {
		return path
	}
	return path[:len(path)-len(ext)]
}

func stripResourceSuffix(path string) string {
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		return path[:idx]
	}
	return path
}

func javaUnquoteLight(raw string) string {
	raw = strings.ReplaceAll(raw, `\/`, `/`)
	raw = strings.ReplaceAll(raw, `\\`, `\`)
	return raw
}
