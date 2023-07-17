package analyzer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	dio "github.com/aquasecurity/go-dep-parser/pkg/io"
	"github.com/aquasecurity/go-dep-parser/pkg/types"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	jarFileRegEx = regexp.MustCompile(`^([a-zA-Z0-9\._-]*[^-*])-(\d\S*(?:-SNAPSHOT)?).jar$`)
)

type JarParser struct {
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

func (p *JarParser) Parse(r dio.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	libs, deps, err := p.parseArtifact(p.rootFilePath, p.size, r)
	if err != nil {
		return nil, nil, utils.Errorf("unable to parse %s: %v", p.rootFilePath, err)
	}
	return removeLibraryDuplicates(libs), deps, nil
}

func (p *JarParser) parseArtifact(filePath string, size int64, r dio.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, nil, utils.Errorf("zip error: %v", err)
	}

	// Try to extract artifactId and version from the file name
	// e.g. spring-core-5.3.4-SNAPSHOT.jar => sprint-core, 5.3.4-SNAPSHOT
	fileProps := parseFileName(filePath)

	var libs []types.Library
	var m manifest
	var foundPomProps bool

	for _, fileInJar := range zr.File {
		switch {
		case filepath.Base(fileInJar.Name) == "pom.properties":
			props, err := parsePomProperties(fileInJar, filePath)
			if err != nil {
				return nil, nil, utils.Errorf("failed to parse %s: %v", fileInJar.Name, err)
			}
			libs = append(libs, props.Library())

			// Check if the pom.properties is for the original JAR/WAR/EAR
			if fileProps.ArtifactID == props.ArtifactID && fileProps.Version == props.Version {
				foundPomProps = true
			}
		case filepath.Base(fileInJar.Name) == "MANIFEST.MF":
			m, err = parseManifest(fileInJar)
			if err != nil {
				return nil, nil, utils.Errorf("failed to parse MANIFEST.MF: %v", err)
			}
		case isArtifact(fileInJar.Name):
			innerLibs, _, err := p.parseInnerJar(fileInJar, filePath) //TODO process inner deps
			if err != nil {
				continue
			}
			libs = append(libs, innerLibs...)
		}
	}

	// If pom.properties is found, it should be preferred than MANIFEST.MF.
	if foundPomProps {
		return libs, nil, nil
	}

	manifestProps := m.properties(filePath)
	if !manifestProps.Valid() {
		return libs, nil, nil
	}
	return append(libs, manifestProps.Library()), nil, nil
}

func (p *JarParser) parseInnerJar(zf *zip.File, rootPath string) ([]types.Library, []types.Dependency, error) {
	fr, err := zf.Open()
	if err != nil {
		return nil, nil, utils.Errorf("unable to open %s: %v", zf.Name, err)
	}

	f, err := os.CreateTemp("", "inner")
	if err != nil {
		return nil, nil, utils.Errorf("unable to create a temp file: %v", err)
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	// Copy the file content to the temp file
	if _, err = io.Copy(f, fr); err != nil {
		return nil, nil, utils.Errorf("file copy error: %v", err)
	}

	// build full path to inner jar
	fullPath := path.Join(rootPath, zf.Name)

	// Parse jar/war/ear recursively
	innerLibs, innerDeps, err := p.parseArtifact(fullPath, int64(zf.UncompressedSize64), f)
	if err != nil {
		return nil, nil, utils.Errorf("failed to parse %s: %v", zf.Name, err)
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

func parseFileName(filePath string) JarProperties {
	fileName := filepath.Base(filePath)
	packageVersion := jarFileRegEx.FindStringSubmatch(fileName)
	if len(packageVersion) != 3 {
		return JarProperties{}
	}

	return JarProperties{
		ArtifactID: packageVersion[1],
		Version:    packageVersion[2],
		FilePath:   filePath,
	}
}

func parsePomProperties(f *zip.File, filePath string) (JarProperties, error) {
	file, err := f.Open()
	if err != nil {
		return JarProperties{}, utils.Errorf("unable to open pom.properties: %v", err)
	}
	defer file.Close()

	p := JarProperties{
		FilePath: filePath,
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
		return JarProperties{}, utils.Errorf("scan error: %v", err)
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
		return manifest{}, utils.Errorf("unable to open MANIFEST.MF: %v", err)
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
			return manifest{}, utils.Errorf("parse MIME header error: %v ", err)
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
