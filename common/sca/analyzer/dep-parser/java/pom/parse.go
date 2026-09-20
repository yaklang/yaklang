package pom

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/model"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	lo "github.com/yaklang/yaklang/common/sca/internal/collection"

	"github.com/yaklang/yaklang/common/sca/core/xmlrecord"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"log"
)

// POM parsing uses only the explicitly supplied snapshot. Repository retrieval,
// settings.xml and environment configuration are deliberately absent.
type parser struct {
	modules  map[string]bool
	ctx      context.Context
	missing  map[string]bool
	rootPath string
	cache    pomCache
	fs       fs.FS
	visiting map[string]bool
	steps    int
	fatal    error
}

func NewParser(filePath string) types.Parser {
	return &parser{rootPath: path.Clean(strings.TrimPrefix(filePath, "/")), cache: newPOMCache(), visiting: map[string]bool{}}
}

func (p *parser) Parse(snapshot fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	p.fs = snapshot
	p.modules = map[string]bool{}
	p.ctx = types.ContextOf(r)
	p.missing = map[string]bool{}
	content, err := parsePom(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse POM: %w", err)
	}

	root := &pom{
		filePath: p.rootPath,
		content:  content,
	}

	// Analyze root POM
	result, err := p.analyze(root, analysisOptions{lineNumber: true})
	if err != nil {
		return nil, nil, fmt.Errorf("analyze error (%s): %w", p.rootPath, err)
	}

	// Cache root POM
	p.cache.put(result.artifact, result)

	libs, deps, err := p.parseRoot(root.artifact())
	if len(libs) > 0 {
		for name := range p.missing {
			libs[0].Diagnostics = append(libs[0].Diagnostics, model.Diagnostic{Code: "evidence_insufficient", Stage: "pom", Reason: name, Incomplete: true})
		}
	} else if len(p.missing) > 0 {
		err = errors.Join(err, fmt.Errorf("evidence_insufficient: POM references %d unavailable materials", len(p.missing)))
	}
	return libs, deps, errors.Join(err, p.fatal)
}

func (p *parser) parseRoot(root artifact) ([]types.Library, []types.Dependency, error) {
	key := root.String()
	if p.modules[key] {
		return nil, nil, fmt.Errorf("malformed_input: POM module cycle %s", key)
	}
	if len(p.modules) >= budget.From(p.ctx).Limits.MaxReferenceDepth {
		return nil, nil, fmt.Errorf("resource_limit: POM module depth")
	}
	if err := p.ctx.Err(); err != nil {
		return nil, nil, err
	}
	p.modules[key] = true
	defer delete(p.modules, key)

	// Prepare a queue for dependencies
	queue := newArtifactQueue()

	// Enqueue root POM
	root.Root = true
	root.Module = false
	queue.enqueue(root)

	var (
		libs              []types.Library
		deps              []types.Dependency
		rootDepManagement []pomDependency
		uniqArtifacts     = map[string]artifact{}
		uniqDeps          = map[string][]string{}
	)

	// Iterate direct and transitive dependencies
	for !queue.IsEmpty() {
		art := queue.dequeue()

		// Modules should be handled separately so that they can have independent dependencies.
		// It means multi-module allows for duplicate dependencies.
		if art.Module {
			moduleLibs, moduleDeps, err := p.parseRoot(art)
			if err != nil {
				return nil, nil, err
			}

			libs = append(libs, moduleLibs...)
			if moduleDeps != nil {
				deps = append(deps, moduleDeps...)
			}
			continue
		}

		// For soft requirements, skip dependency resolution that has already been resolved.
		if uniqueArt, ok := uniqArtifacts[art.Name()]; ok {
			if !uniqueArt.Version.shouldOverride(art.Version) {
				continue
			}
			// mark artifact as Direct, if saved artifact is Direct
			// take a look `hard requirement for the specified version` test
			if uniqueArt.Direct {
				art.Direct = true
			}
			// We don't need to overwrite dependency location for hard links
			if uniqueArt.Locations != nil {
				art.Locations = uniqueArt.Locations
			}
		}

		result, err := p.resolve(art, rootDepManagement)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve error (%s): %w", art, err)
		}

		if art.Root {
			// Managed dependencies in the root POM affect transitive dependencies
			rootDepManagement = p.resolveDepManagement(result.properties, result.dependencyManagement)

			// mark root artifact and its dependencies as Direct
			art.Direct = true
			result.dependencies = lo.Map(result.dependencies, func(dep artifact, _ int) artifact {
				dep.Direct = true
				return dep
			})
		}

		// Parse, cache, and enqueue modules.
		for _, relativePath := range result.modules {
			moduleArtifact, err := p.parseModule(result.filePath, relativePath)
			if err != nil {
				p.missing[fmt.Sprintf("module %s: %v", relativePath, err)] = true
				continue
			}

			queue.enqueue(moduleArtifact)
		}

		// Resolve transitive dependencies later
		queue.enqueue(result.dependencies...)

		// Offline mode may be missing some fields.
		if !art.IsEmpty() {
			// Override the version
			uniqArtifacts[art.Name()] = artifact{
				Version:   art.Version,
				Licenses:  result.artifact.Licenses,
				Direct:    art.Direct,
				Root:      art.Root,
				Locations: art.Locations,
			}

			// save only dependency names
			// version will be determined later
			dependsOn := lo.Map(result.dependencies, func(a artifact, _ int) string {
				return a.Name()
			})
			uniqDeps[packageID(art.Name(), art.Version.String())] = dependsOn
		}
	}

	// Convert to []types.Library and []types.Dependency
	for name, art := range uniqArtifacts {
		lib := types.Library{
			ID:       packageID(name, art.Version.String()),
			Evidence: "declared", DeclaredName: name, DeclaredVersion: art.Version.String(), IsVersionRange: strings.ContainsAny(art.Version.String(), "[](),${}"),
			Name:      name,
			Version:   art.Version.String(),
			License:   art.JoinLicenses(),
			Indirect:  !art.Direct,
			Locations: art.Locations,
		}
		libs = append(libs, lib)

		// Convert dependency names into dependency IDs
		dependsOn := lo.FilterMap(uniqDeps[lib.ID], func(dependOnName string, _ int) (string, bool) {
			ver := depVersion(dependOnName, uniqArtifacts)
			return packageID(dependOnName, ver), ver != ""
		})

		sort.Strings(dependsOn)
		if len(dependsOn) > 0 {
			deps = append(deps, types.Dependency{
				ID:        lib.ID,
				DependsOn: dependsOn,
			})
		}
	}

	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))

	return libs, deps, nil
}

// depVersion finds dependency in uniqArtifacts and return its version
func depVersion(depName string, uniqArtifacts map[string]artifact) string {
	if art, ok := uniqArtifacts[depName]; ok {
		return art.Version.String()
	}
	return ""
}

func (p *parser) parseModule(currentPath, relativePath string) (artifact, error) {
	// modulePath: "root/" + "module/" => "root/module"
	module, err := p.openRelativePom(currentPath, relativePath)
	if err != nil {
		return artifact{}, fmt.Errorf("unable to open the relative path: %w", err)
	}

	result, err := p.analyze(module, analysisOptions{})
	if err != nil {
		return artifact{}, fmt.Errorf("analyze error: %w", err)
	}

	moduleArtifact := module.artifact()
	moduleArtifact.Module = true

	p.cache.put(moduleArtifact, result)

	return moduleArtifact, nil
}

func (p *parser) resolve(art artifact, rootDepManagement []pomDependency) (analysisResult, error) {
	// If the artifact is found in cache, it is returned.
	if result := p.cache.get(art); result != nil {
		return *result, nil
	}

	log.Printf("Resolving %s:%s:%s...", art.GroupID, art.ArtifactID, art.Version)
	pomContent, err := p.tryRepository(art.GroupID, art.ArtifactID, art.Version.String())
	if err != nil {
		log.Print(err)
	}
	result, err := p.analyze(pomContent, analysisOptions{
		exclusions:    art.Exclusions,
		depManagement: rootDepManagement,
	})
	if err != nil {
		return analysisResult{}, fmt.Errorf("analyze error: %w", err)
	}

	p.cache.put(art, result)
	return result, nil
}

type analysisResult struct {
	filePath             string
	artifact             artifact
	dependencies         []artifact
	dependencyManagement []pomDependency // Keep the order of dependencies in 'dependencyManagement'
	properties           map[string]string
	modules              []string
}

type analysisOptions struct {
	exclusions    map[string]struct{}
	depManagement []pomDependency // from the root POM
	lineNumber    bool            // Save line numbers
}

func (p *parser) analyze(pom *pom, opts analysisOptions) (analysisResult, error) {
	if pom == nil || pom.content == nil {
		return analysisResult{}, nil
	}

	p.steps++
	l := budget.From(p.ctx).Limits
	if err := p.ctx.Err(); err != nil {
		return analysisResult{}, err
	}
	if p.steps > l.MaxResolveSteps || len(p.visiting) >= l.MaxReferenceDepth {
		p.fatal = errors.New("resource_limit: POM resolution budget exceeded")
		return analysisResult{}, p.fatal
	}
	if p.visiting[pom.filePath] {
		p.fatal = fmt.Errorf("POM reference cycle: %s", pom.filePath)
		return analysisResult{}, p.fatal
	}
	p.visiting[pom.filePath] = true
	defer delete(p.visiting, pom.filePath)

	// Parent
	parent, err := p.parseParent(pom.filePath, pom.content.Parent)
	if err != nil {
		return analysisResult{}, fmt.Errorf("parent error: %w", err)
	}

	// Inherit values/properties from parent
	pom.inherit(parent)

	// Generate properties
	props := pom.properties()

	// dependencyManagements have the next priority:
	// 1. Managed dependencies from this POM
	// 2. Managed dependencies from parent of this POM
	depManagement := p.mergeDependencyManagements(pom.content.DependencyManagement.Dependencies.Dependency,
		parent.dependencyManagement)

	// Merge dependencies. Child dependencies must be preferred than parent dependencies.
	// Parents don't have to resolve dependencies.
	deps := p.parseDependencies(pom.content.Dependencies.Dependency, props, depManagement, opts)
	deps = p.mergeDependencies(parent.dependencies, deps, opts.exclusions)

	return analysisResult{
		filePath:             pom.filePath,
		artifact:             pom.artifact(),
		dependencies:         deps,
		dependencyManagement: depManagement,
		properties:           props,
		modules:              pom.content.Modules.Module,
	}, nil
}

func (p *parser) mergeDependencyManagements(depManagements ...[]pomDependency) []pomDependency {
	uniq := map[string]struct{}{}
	var depManagement []pomDependency
	// The preceding argument takes precedence.
	for _, dm := range depManagements {
		for _, dep := range dm {
			if _, ok := uniq[dep.Name()]; ok {
				continue
			}
			depManagement = append(depManagement, dep)
			uniq[dep.Name()] = struct{}{}
		}
	}
	return depManagement
}

func (p *parser) parseDependencies(deps []pomDependency, props map[string]string, depManagement []pomDependency,
	opts analysisOptions,
) []artifact {
	// Imported POMs often have no dependencies, so dependencyManagement resolution can be skipped.
	if len(deps) == 0 {
		return nil
	}

	// Resolve dependencyManagement
	depManagement = p.resolveDepManagement(props, depManagement)

	rootDepManagement := opts.depManagement
	var dependencies []artifact
	for _, d := range deps {
		// Resolve dependencies
		d = d.Resolve(props, depManagement, rootDepManagement)
		if strings.Contains(d.Version, "${") {
			p.missing["unresolved property: "+d.Version] = true
		}
		dependencies = append(dependencies, d.ToArtifact(opts))
	}
	return dependencies
}

func (p *parser) resolveDepManagement(props map[string]string, depManagement []pomDependency) []pomDependency {
	var newDepManagement, imports []pomDependency
	for _, dep := range depManagement {
		// cf. https://howtodoinjava.com/maven/maven-dependency-scopes/#import
		if dep.Scope == "import" {
			imports = append(imports, dep)
		} else {
			// Evaluate variables
			newDepManagement = append(newDepManagement, dep.Resolve(props, nil, nil))
		}
	}

	// Managed dependencies with a scope of "import" should be processed after other managed dependencies.
	// cf. https://maven.apache.org/guides/introduction/introduction-to-dependency-mechanism.html#importing-dependencies
	for _, imp := range imports {
		art := newArtifact(imp.GroupID, imp.ArtifactID, imp.Version, nil, props)
		result, err := p.resolve(art, nil)
		if err != nil {
			continue
		}

		// We need to recursively check all nested depManagements,
		// so that we don't miss dependencies on nested depManagements with `Import` scope.
		newProps := utils.MergeMaps(props, result.properties)
		result.dependencyManagement = p.resolveDepManagement(newProps, result.dependencyManagement)
		for k, dd := range result.dependencyManagement {
			// Evaluate variables and overwrite dependencyManagement
			result.dependencyManagement[k] = dd.Resolve(newProps, nil, nil)
		}
		newDepManagement = p.mergeDependencyManagements(newDepManagement, result.dependencyManagement)
	}
	return newDepManagement
}

func (p *parser) mergeDependencies(parent, child []artifact, exclusions map[string]struct{}) []artifact {
	var deps []artifact
	unique := map[string]struct{}{}

	for _, d := range append(child, parent...) {
		if excludeDep(exclusions, d) {
			continue
		}
		if _, ok := unique[d.Name()]; ok {
			continue
		}
		unique[d.Name()] = struct{}{}
		deps = append(deps, d)
	}

	return deps
}

func excludeDep(exclusions map[string]struct{}, art artifact) bool {
	if _, ok := exclusions[art.Name()]; ok {
		return true
	}
	// Maven can use "*" in GroupID and ArtifactID fields to exclude dependencies
	// https://maven.apache.org/pom.html#exclusions
	for exlusion := range exclusions {
		// exclusion format - "<groupID>:<artifactID>"
		e := strings.Split(exlusion, ":")
		if (e[0] == art.GroupID || e[0] == "*") && (e[1] == art.ArtifactID || e[1] == "*") {
			return true
		}
	}
	return false
}

func (p *parser) parseParent(currentPath string, parent pomParent) (analysisResult, error) {
	// Pass nil properties so that variables in <parent> are not evaluated.
	target := newArtifact(parent.GroupId, parent.ArtifactId, parent.Version, nil, nil)
	// if version is property (e.g. ${revision}) - we still need to parse this pom
	if target.IsEmpty() && !isProperty(parent.Version) {
		return analysisResult{}, nil
	}
	log.Printf("Start parent: %s", target.String())
	defer func() {
		log.Printf("Exit parent: %s", target.String())
	}()

	// If the artifact is found in cache, it is returned.
	if result := p.cache.get(target); result != nil {
		return *result, nil
	}

	parentPOM, err := p.retrieveParent(currentPath, parent.RelativePath, target)
	if err != nil {
		log.Printf("parent POM not found: %s", err)
	}

	if parentPOM == nil {
		return analysisResult{artifact: target}, nil
	}
	result, err := p.analyze(parentPOM, analysisOptions{})
	if err != nil {
		return analysisResult{}, fmt.Errorf("analyze error: %w", err)
	}

	p.cache.put(target, result)

	return result, nil
}

func (p *parser) retrieveParent(currentPath, relativePath string, target artifact) (*pom, error) {
	var errs error

	// Try relativePath
	if relativePath != "" {
		pom, err := p.tryRelativePath(target, currentPath, relativePath)
		if err != nil {
			errs = errors.Join(errs, err)
		} else {
			return pom, nil
		}
	}

	// If not found, search the parent director
	pom, err := p.tryRelativePath(target, currentPath, "../pom.xml")
	if err != nil {
		errs = errors.Join(errs, err)
	} else {
		return pom, nil
	}

	// Only the caller supplied repository tree may satisfy a coordinate.
	doc, err := p.tryRepository(target.GroupID, target.ArtifactID, target.Version.String())
	if err == nil {
		return doc, nil
	}
	return nil, errors.Join(errs, err)
}

func (p *parser) tryRelativePath(parentArtifact artifact, currentPath, relativePath string) (*pom, error) {
	pom, err := p.openRelativePom(currentPath, relativePath)
	if err != nil {
		return nil, err
	}

	// To avoid an infinite loop or parsing the wrong parent when using relatedPath or `../pom.xml`,
	// we need to compare GAV of `parentArtifact` (`parent` tag from base pom) and GAV of pom from `relativePath`.
	// See `compare ArtifactIDs for base and parent pom's` test for example.
	// But GroupID can be inherited from parent (`p.analyze` function is required to get the GroupID).
	// Version can contain a property (`p.analyze` function is required to get the GroupID).
	// So we can only match ArtifactID's.
	if pom.artifact().ArtifactID != parentArtifact.ArtifactID {
		return nil, errors.New("'parent.relativePath' points at wrong local POM")
	}
	result, err := p.analyze(pom, analysisOptions{})
	if err != nil {
		return nil, fmt.Errorf("analyze error: %w", err)
	}

	expected := parentArtifact
	expected.Version = newVersion(evaluateVariable(parentArtifact.Version.String(), result.properties, nil))
	if !expected.Equal(result.artifact) {
		return nil, errors.New("'parent.relativePath' points at wrong local POM")
	}

	return pom, nil
}

func (p *parser) openRelativePom(currentPath, relativePath string) (*pom, error) {
	// e.g. child/pom.xml => child/
	dir := filepath.Dir(currentPath)

	// e.g. child + ../parent => parent/
	filePath := filepath.Join(dir, relativePath)

	isDir, err := p.isDirectory(filePath)
	if err != nil {
		return nil, err
	} else if isDir {
		// e.g. parent/ => parent/pom.xml
		filePath = filepath.Join(filePath, "pom.xml")
	}

	pom, err := p.openPom(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	return pom, nil
}

func (p *parser) openPom(filePath string) (*pom, error) {
	if p.fs == nil || !fs.ValidPath(filePath) || strings.ContainsAny(filePath, `\:`) {
		return nil, fmt.Errorf("POM reference outside snapshot: %s", filePath)
	}
	f, err := p.fs.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("file open error (%s): %w", filePath, err)
	}

	defer f.Close()
	content, err := parsePom(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the local POM: %w", err)
	}
	return &pom{
		filePath: filePath,
		content:  content,
	}, nil
}

// The only repository layout is an explicitly supplied snapshot directory.
// There is no repository configuration, network endpoint, or host cache fallback.
func (p *parser) tryRepository(groupID, artifactID, version string) (*pom, error) {
	if groupID != "" && artifactID != "" && version != "" && !strings.ContainsAny(groupID+artifactID+version, `\/:$[](), `) {
		name := path.Join("repository", strings.ReplaceAll(groupID, ".", "/"), artifactID, version, artifactID+"-"+version+".pom")
		if doc, err := p.openPom(name); err == nil {
			return doc, nil
		}
	}
	p.missing[fmt.Sprintf("POM metadata absent from snapshot: %s:%s:%s", groupID, artifactID, version)] = true
	return nil, fmt.Errorf("evidence_insufficient: POM %s:%s:%s is not in the supplied snapshot", groupID, artifactID, version)
}
func (p *parser) isDirectory(name string) (bool, error) {
	if p.fs == nil || !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
		return false, fs.ErrPermission
	}
	info, err := fs.Stat(p.fs, name)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func parsePom(r io.Reader) (*pomXML, error) {
	parsed := &pomXML{}
	if err := xmlrecord.Decode(types.ContextOf(r), r, parsed); err != nil {
		return nil, fmt.Errorf("xml decode error: %w", err)
	}
	return parsed, nil
}
