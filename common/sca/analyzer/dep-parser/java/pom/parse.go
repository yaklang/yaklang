package pom

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/model"
	"io"
	"io/fs"
	"path"
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
	modules     map[string]bool
	ctx         context.Context
	missing     map[string]bool
	rootPath    string
	cache       pomCache
	fs          fs.FS
	visiting    map[string]bool
	bomVisiting []string
	bomResolved map[string][]pomDependency
	steps       int
	fatal       error
}

func NewParser(filePath string) types.Parser {
	return &parser{rootPath: path.Clean(strings.TrimPrefix(filePath, "/")), cache: newPOMCache(), visiting: map[string]bool{}, bomResolved: map[string][]pomDependency{}}
}

func (p *parser) Parse(snapshot fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	p.fs = snapshot
	p.modules = map[string]bool{}
	p.ctx = types.ContextOf(r)
	p.missing = map[string]bool{}
	p.visiting = map[string]bool{}
	p.bomVisiting = nil
	p.bomResolved = map[string][]pomDependency{}
	p.steps = 0
	p.fatal = nil
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

	if err := p.remember(result.artifact, result); err != nil {
		return nil, nil, err
	}

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
		uniqDeps          = map[string][]pomEdge{}
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
			rootDepManagement, err = p.resolveDepManagement(result.properties, result.dependencyManagement)
			if err != nil {
				return nil, nil, err
			}

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
				Version:    art.Version,
				Licenses:   result.artifact.Licenses,
				Direct:     art.Direct,
				Root:       art.Root,
				Locations:  art.Locations,
				Type:       art.Type,
				Classifier: art.Classifier,
			}

			uniqDeps[packageID(art.Name(), art.Version.String())] = lo.Map(result.dependencies, func(a artifact, _ int) pomEdge {
				return pomEdge{Name: a.Name(), Constraint: a.DeclaredConstraint, Version: a.Version.String()}
			})
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

		var dependsOn []string
		var reqs []types.Requirement
		for _, edge := range uniqDeps[lib.ID] {
			req := types.Requirement{Target: edge.Name, Constraint: edge.Constraint, Scope: "runtime"}
			if pomVersionKnown(edge.Version) {
				id := packageID(edge.Name, edge.Version)
				dependsOn = append(dependsOn, id)
				req.Resolved = id
			}
			reqs = append(reqs, req)
		}

		sort.Strings(dependsOn)
		sort.Slice(reqs, func(i, j int) bool { return reqs[i].Target < reqs[j].Target })
		if len(dependsOn) > 0 || len(reqs) > 0 {
			deps = append(deps, types.Dependency{
				ID:           lib.ID,
				DependsOn:    dependsOn,
				Requirements: reqs,
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

type pomEdge struct {
	Name, Constraint, Version string
}

func pomVersionKnown(v string) bool {
	return v != "" && !strings.ContainsAny(v, "[](),${}")
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

	if err := p.remember(moduleArtifact, result); err != nil {
		return artifact{}, err
	}

	return moduleArtifact, nil
}

func (p *parser) charge() error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	p.steps++
	if p.steps > budget.From(p.ctx).Limits.MaxResolveSteps {
		err := scanerr.New(scanerr.ResourceLimit, "POM resolution budget exceeded")
		p.fatal = err
		return err
	}
	return nil
}

func (p *parser) resolve(art artifact, rootDepManagement []pomDependency) (analysisResult, error) {
	if err := p.charge(); err != nil {
		return analysisResult{}, err
	}
	// Cached analysis is not a safe expansion of imported BOM closures.
	// Shared POM material was charged when first stored.
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

	if err := p.remember(art, result); err != nil {
		return analysisResult{}, err
	}
	return result, nil
}

func (p *parser) remember(art artifact, result analysisResult) error {
	key := p.cache.key(art)
	if err := budget.From(p.ctx).Once("pom:"+key, 1, budget.SizeObject*4+budget.SizeOfString(key)); err != nil {
		p.fatal = err
		return err
	}
	p.cache.put(art, result)
	return nil
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
	deps, err := p.parseDependencies(pom.content.Dependencies.Dependency, props, depManagement, opts)
	if err != nil {
		return analysisResult{}, err
	}
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
) ([]artifact, error) {
	// Imported POMs often have no dependencies, so dependencyManagement resolution can be skipped.
	if len(deps) == 0 {
		return nil, nil
	}

	// Resolve dependencyManagement
	var err error
	depManagement, err = p.resolveDepManagement(props, depManagement)
	if err != nil {
		return nil, err
	}

	rootDepManagement := opts.depManagement
	var dependencies []artifact
	for _, d := range deps {
		declared := d.Version
		d = d.Resolve(props, depManagement, rootDepManagement)
		if strings.Contains(d.Version, "${") {
			p.missing["unresolved property: "+d.Version] = true
		}
		art := d.ToArtifact(opts)
		art.DeclaredConstraint = declared
		dependencies = append(dependencies, art)
	}
	return dependencies, nil
}

func (p *parser) resolveDepManagement(props map[string]string, depManagement []pomDependency) ([]pomDependency, error) {
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
		if err := p.charge(); err != nil {
			return nil, err
		}
		art := newArtifact(imp.GroupID, imp.ArtifactID, imp.Version, nil, props)
		key := p.cache.key(art)
		for i, seen := range p.bomVisiting {
			if seen == key {
				path := append(append([]string{}, p.bomVisiting[i:]...), key)
				err := scanerr.New(scanerr.MalformedInput, "POM BOM import cycle: %s", strings.Join(path, " -> "))
				p.fatal = err
				return nil, err
			}
		}
		if len(p.bomVisiting) >= budget.From(p.ctx).Limits.MaxReferenceDepth {
			err := scanerr.New(scanerr.ResourceLimit, "POM BOM import depth")
			p.fatal = err
			return nil, err
		}
		if expanded, ok := p.bomResolved[key]; ok {
			newDepManagement = p.mergeDependencyManagements(newDepManagement, expanded)
			continue
		}
		p.bomVisiting = append(p.bomVisiting, key)
		result, err := p.resolve(art, nil)
		if err != nil {
			p.bomVisiting = p.bomVisiting[:len(p.bomVisiting)-1]
			return nil, err
		}
		newProps := utils.MergeMaps(props, result.properties)
		expanded, err := p.resolveDepManagement(newProps, result.dependencyManagement)
		p.bomVisiting = p.bomVisiting[:len(p.bomVisiting)-1]
		if err != nil {
			return nil, err
		}
		for k, dd := range expanded {
			expanded[k] = dd.Resolve(newProps, nil, nil)
		}
		p.bomResolved[key] = expanded
		newDepManagement = p.mergeDependencyManagements(newDepManagement, expanded)
	}
	return newDepManagement, nil
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

	if err := p.remember(target, result); err != nil {
		return analysisResult{}, err
	}

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
	dir := path.Dir(currentPath)

	// e.g. child + ../parent => parent/
	filePath := path.Join(dir, relativePath)

	isDir, err := p.isDirectory(filePath)
	if err != nil {
		return nil, err
	} else if isDir {
		// e.g. parent/ => parent/pom.xml
		filePath = path.Join(filePath, "pom.xml")
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
