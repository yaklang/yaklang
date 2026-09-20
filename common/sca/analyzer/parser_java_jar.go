package analyzer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"net/textproto"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	lo "github.com/yaklang/yaklang/common/sca/internal/collection"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type JarParser struct {
	ctx    context.Context
	budget *budget.State
	depth  int

	rootFilePath string
	offline      bool
	size         int64
}

type JarProperties struct {
	GroupID    string
	ArtifactID string
	Version    string
	FilePath   string // path to file containing these props
}

func (p JarProperties) Library() types.Library {
	return types.Library{
		Name:     fmt.Sprintf("%s:%s", p.GroupID, p.ArtifactID),
		Version:  p.Version,
		FilePath: p.FilePath,
	}
}

func (p JarProperties) Valid() bool {
	return p.GroupID != "" && p.ArtifactID != "" && p.Version != ""
}

func (p JarProperties) String() string {
	return fmt.Sprintf("%s:%s:%s", p.GroupID, p.ArtifactID, p.Version)
}

func NewJarParser(path string, size int64) types.Parser {
	return &JarParser{
		rootFilePath: path,
		size:         size,
	}
}

func (p *JarParser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	p.ctx = types.ContextOf(r)
	p.budget = budget.From(p.ctx)
	libs, deps, err := p.parseArtifact(fs, p.rootFilePath, p.size, r)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse %s: %v", p.rootFilePath, err)
	}
	return removeLibraryDuplicates(libs), deps, nil
}

func (p *JarParser) parseArtifact(fs fi.FileSystem, filePath string, size int64, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > p.budget.Limits.MaxArchiveDepth {
		return nil, nil, fmt.Errorf("resource_limit: archive depth")
	}
	if err := p.ctx.Err(); err != nil {
		return nil, nil, err
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, nil, fmt.Errorf("zip error: %v", err)
	}

	var libs []types.Library
	var m manifest
	var manifestPath string
	var explicitCoordinates bool

	if err := p.budget.Archive(len(zr.File), 0); err != nil {
		return nil, nil, err
	}
	for _, fileInJar := range zr.File {
		if err := p.ctx.Err(); err != nil {
			return nil, nil, err
		}
		entryName := strings.TrimSuffix(fileInJar.Name, "/")
		if !iofs.ValidPath(entryName) || strings.ContainsAny(entryName, `\:`) {
			return nil, nil, fmt.Errorf("invalid_path: archive entry %q", fileInJar.Name)
		}
		if fileInJar.Mode()&iofs.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("unsupported_syntax: archive symlink")
		}
		if fileInJar.UncompressedSize64 > uint64(p.budget.Limits.MaxFileBytes) {
			return nil, nil, fmt.Errorf("resource_limit: archive entry size")
		}
		if err := p.budget.Archive(0, int64(fileInJar.UncompressedSize64)); err != nil {
			return nil, nil, err
		}
		filename := filepath.Base(fileInJar.Name)
		switch {
		case filename == "pom.properties":
			if fileInJar.UncompressedSize64 > uint64(p.budget.Limits.MaxFieldBytes) {
				return nil, nil, fmt.Errorf("resource_limit: pom.properties")
			}
			props, err := parsePomProperties(fileInJar, filePath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse %s: %v", fileInJar.Name, err)
			}
			if !props.Valid() {
				return nil, nil, fmt.Errorf("malformed_input: incomplete pom.properties identity")
			}
			lib := props.Library()
			if err := p.budget.Result(budget.SizeOfPackage(lib.Name, lib.Version, lib.FilePath)); err != nil {
				return nil, nil, err
			}
			libs = append(libs, lib)
			explicitCoordinates = true
		case filename == "MANIFEST.MF":
			if fileInJar.UncompressedSize64 > uint64(p.budget.Limits.MaxFieldBytes) {
				return nil, nil, fmt.Errorf("resource_limit: manifest")
			}
			manifestPath = filePath + "!/" + fileInJar.Name
			m, err = parseManifest(fileInJar)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse MANIFEST.MF: %v", err)
			}
		case isArtifact(fileInJar.Name):
			innerLibs, _, err := p.parseInnerJar(fs, fileInJar, filePath) // TODO process inner deps
			if err != nil {
				return nil, nil, err
			}
			libs = append(libs, innerLibs...)
		}
	}

	// Explicit Maven metadata takes precedence over vendor/title guesses. File
	// names play no part in this decision, including shaded and renamed archives.
	if explicitCoordinates {
		return libs, nil, nil
	}
	manifestProps := m.properties(manifestPath)
	if !manifestProps.Valid() {
		return libs, nil, nil
	}
	candidate := manifestProps.Library()
	for _, l := range libs {
		if l.Name == candidate.Name && l.Version == candidate.Version {
			return libs, nil, nil
		}
	}
	// Manifest vendor/title fallbacks do not prove Maven coordinates.
	candidate.Evidence = "inferred"
	if err := p.budget.Result(budget.SizeOfPackage(candidate.Name, candidate.Version, candidate.FilePath)); err != nil {
		return nil, nil, err
	}
	return append(libs, candidate), nil, nil
}

func (p *JarParser) parseInnerJar(fs fi.FileSystem, zf *zip.File, rootPath string) ([]types.Library, []types.Dependency, error) {
	fr, err := zf.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to open %s: %v", zf.Name, err)
	}

	defer fr.Close()
	data, err := io.ReadAll(io.LimitReader(fr, p.budget.Limits.MaxFileBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > p.budget.Limits.MaxFileBytes {
		return nil, nil, fmt.Errorf("resource_limit: expanded archive")
	}
	if err := p.budget.Working(int64(len(data))); err != nil {
		return nil, nil, err
	}
	f := bytes.NewReader(data)

	// build full path to inner jar
	fullPath := rootPath + "!/" + zf.Name

	// Parse jar/war/ear recursively
	innerLibs, innerDeps, err := p.parseArtifact(fs, fullPath, int64(zf.UncompressedSize64), f)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse %s: %v", zf.Name, err)
	}

	return innerLibs, innerDeps, nil
}

func isArtifact(name string) bool {
	ext := filepath.Ext(name)
	if ext == ".jar" || ext == ".ear" || ext == ".war" {
		return true
	}
	return false
}

func parsePomProperties(f *zip.File, filePath string) (JarProperties, error) {
	file, err := f.Open()
	if err != nil {
		return JarProperties{}, fmt.Errorf("unable to open pom.properties: %v", err)
	}
	defer file.Close()

	p := JarProperties{
		FilePath: filePath + "!/" + f.Name,
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "groupId="):
			p.GroupID = strings.TrimPrefix(line, "groupId=")
		case strings.HasPrefix(line, "artifactId="):
			p.ArtifactID = strings.TrimPrefix(line, "artifactId=")
		case strings.HasPrefix(line, "version="):
			p.Version = strings.TrimPrefix(line, "version=")
		}
	}

	if err = scanner.Err(); err != nil {
		return JarProperties{}, fmt.Errorf("scan error: %v", err)
	}
	return p, nil
}

type manifest struct {
	implementationVersion  string
	implementationTitle    string
	implementationVendor   string
	implementationVendorId string
	specificationTitle     string
	specificationVersion   string
	specificationVendor    string
	bundleName             string
	bundleVersion          string
	bundleSymbolicName     string
}

func parseManifest(f *zip.File) (manifest, error) {
	file, err := f.Open()
	if err != nil {
		return manifest{}, fmt.Errorf("unable to open MANIFEST.MF: %v", err)
	}
	defer file.Close()

	var m manifest

	br := bufio.NewReader(file)
	for {
		block, err := ReadBlock(br)
		if err != nil && err != io.EOF {
			return manifest{}, err
		}
		if block == nil {
			break
		}
		reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(block)))
		header, err := reader.ReadMIMEHeader()
		if err != nil && err != io.EOF {
			return manifest{}, fmt.Errorf("parse MIME header error: %v ", err)
		}
		m.implementationVersion = header.Get("Implementation-Version")
		m.implementationTitle = header.Get("Implementation-Title")
		m.implementationVendor = header.Get("Implementation-Vendor")
		m.implementationVendorId = header.Get("Implementation-Vendor-Id")
		m.specificationVersion = header.Get("Specification-Version")
		m.specificationTitle = header.Get("Specification-Title")
		m.specificationVendor = header.Get("Specification-Vendor")
		m.bundleVersion = header.Get("Bundle-Version")
		m.bundleName = header.Get("Bundle-Name")
		m.bundleSymbolicName = header.Get("Bundle-SymbolicName")

		// only parse the first block
		break
	}
	return m, nil
}

func (m manifest) properties(filePath string) JarProperties {
	groupID, err := m.determineGroupID()
	if err != nil {
		return JarProperties{}
	}

	artifactID, err := m.determineArtifactID()
	if err != nil {
		return JarProperties{}
	}

	version, err := m.determineVersion()
	if err != nil {
		return JarProperties{}
	}

	return JarProperties{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
		FilePath:   filePath,
	}
}

func (m manifest) determineGroupID() (string, error) {
	var groupID string
	switch {
	case m.bundleSymbolicName != "":
		groupID = m.bundleSymbolicName
		// e.g. "com.fasterxml.jackson.core.jackson-databind" => "com.fasterxml.jackson.core"
		idx := strings.LastIndex(m.bundleSymbolicName, ".")
		if idx > 0 {
			groupID = m.bundleSymbolicName[:idx]
		}
	case m.implementationVendorId != "":
		groupID = m.implementationVendorId
	case m.implementationVendor != "":
		groupID = m.implementationVendor
	case m.specificationVendor != "":
		groupID = m.specificationVendor
	default:
		return "", errors.New("no groupID found")
	}
	return strings.TrimSpace(groupID), nil
}

func (m manifest) determineArtifactID() (string, error) {
	var artifactID string
	switch {
	case m.bundleName != "":
		artifactID = m.bundleName
	case m.implementationTitle != "":
		artifactID = m.implementationTitle
	case m.specificationTitle != "":
		artifactID = m.specificationTitle
	default:
		return "", errors.New("no artifactID found")
	}
	return strings.TrimSpace(artifactID), nil
}

func (m manifest) determineVersion() (string, error) {
	var version string
	switch {
	case m.bundleVersion != "":
		version = m.bundleVersion
	case m.implementationVersion != "":
		version = m.implementationVersion
	case m.specificationVersion != "":
		version = m.specificationVersion
	default:
		return "", errors.New("no version found")
	}
	return strings.TrimSpace(version), nil
}

func removeLibraryDuplicates(libs []types.Library) []types.Library {
	return lo.UniqBy(libs, func(lib types.Library) string {
		return fmt.Sprintf("%s::%s::%s", lib.Name, lib.Version, lib.FilePath)
	})
}
