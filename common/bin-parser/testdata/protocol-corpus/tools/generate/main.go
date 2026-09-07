// Command generate downloads pinned upstream captures and builds the protocol
// corpus manifest, representative frame digests and coverage reports.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/bin-parser/internal/corpusutil"
)

type sourceSpec struct {
	Schema        string           `json:"$schema,omitempty"`
	SchemaVersion int              `json:"schema_version"`
	Repositories  []repositorySpec `json:"repositories"`
	Captures      []captureSpec    `json:"captures"`
}

type repositorySpec struct {
	ID            string `json:"id"`
	Repository    string `json:"repository"`
	Commit        string `json:"commit"`
	License       string `json:"license"`
	LicensePath   string `json:"license_path"`
	LicenseSHA256 string `json:"license_sha256"`
	Homepage      string `json:"homepage,omitempty"`
	Origin        string `json:"origin,omitempty"`
	Recipe        string `json:"recipe,omitempty"`
}

type captureSpec struct {
	ID            string   `json:"id"`
	RepositoryID  string   `json:"repository_id"`
	UpstreamPath  string   `json:"upstream_path"`
	Protocol      string   `json:"protocol"`
	RoadmapName   *string  `json:"roadmap_name"`
	DisplayFilter string   `json:"display_filter"`
	DecodeAs      []string `json:"decode_as,omitempty"`
	EvidenceKind  string   `json:"evidence_kind"`
	Notes         string   `json:"notes,omitempty"`
	AllowEmpty    bool     `json:"allow_empty,omitempty"`
	SourceSHA256  string   `json:"source_sha256"`
}

type manifest struct {
	SchemaVersion int                  `json:"schema_version"`
	RoadmapTotal  int                  `json:"roadmap_total"`
	Repositories  []manifestRepository `json:"repositories"`
	Captures      []manifestCapture    `json:"captures"`
}

type manifestRepository struct {
	ID            string `json:"id"`
	Repository    string `json:"repository"`
	Commit        string `json:"commit"`
	License       string `json:"license"`
	Homepage      string `json:"homepage,omitempty"`
	LicenseFile   string `json:"license_file"`
	LicenseSHA256 string `json:"license_sha256"`
}

type manifestCapture struct {
	ID                  string               `json:"id"`
	RepositoryID        string               `json:"repository_id"`
	Protocol            string               `json:"protocol"`
	RoadmapName         *string              `json:"roadmap_name"`
	RoadmapFamily       *string              `json:"roadmap_family"`
	RoadmapPriority     *string              `json:"roadmap_priority"`
	RoadmapStatus       *string              `json:"roadmap_status"`
	CaptureFile         string               `json:"capture_file"`
	SourceURL           string               `json:"source_url"`
	UpstreamPath        string               `json:"upstream_path"`
	SHA256              string               `json:"sha256"`
	SizeBytes           int64                `json:"size_bytes"`
	PacketCount         int                  `json:"packet_count"`
	LinkType            string               `json:"link_type"`
	DisplayFilter       string               `json:"display_filter"`
	DecodeAs            []string             `json:"decode_as,omitempty"`
	EvidenceKind        string               `json:"evidence_kind"`
	Notes               string               `json:"notes,omitempty"`
	RepresentativeFrame *representativeFrame `json:"representative_frame"`
	FrameProtocols      []string             `json:"frame_protocols"`
}

type representativeFrame struct {
	Number      int    `json:"number"`
	LengthBytes int    `json:"length_bytes"`
	SHA256      string `json:"sha256"`
}

type roadmapItem struct {
	Name     string
	Family   string
	Status   string
	Priority string
}

type packetDataReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
}

var (
	roadmapPattern    = regexp.MustCompile(`\{Name: "([^"]+)", Family: "([^"]+)".*Status: (st\w+), Priority: (pri\w+)`)
	sourceIDPattern   = regexp.MustCompile(`^[a-z0-9-]+$`)
	repositoryPattern = regexp.MustCompile(`^[^/]+/[^/]+$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	decodeAsPattern   = regexp.MustCompile(`^[a-z][a-z0-9_.]*==[0-9]+,[a-z][a-z0-9_.-]*$`)
)

func main() {
	fetch := flag.Bool("fetch", false, "download pinned captures and license texts before generating")
	flag.Parse()

	repoRoot, err := findRepoRoot()
	must(err)
	corpusDir := filepath.Join(repoRoot, "common", "bin-parser", "testdata", "protocol-corpus")

	var spec sourceSpec
	must(readJSON(filepath.Join(corpusDir, "sources.json"), &spec))
	must(validateSpec(spec))

	roadmap, err := readRoadmap(filepath.Join(repoRoot, "common", "bin-parser", "protocol_roadmap.go"))
	must(err)
	roadmapByName := make(map[string]roadmapItem, len(roadmap))
	for _, item := range roadmap {
		roadmapByName[item.Name] = item
	}

	repositories, repoByID, err := prepareRepositories(corpusDir, spec.Repositories, *fetch)
	must(err)
	captures, err := prepareCaptures(corpusDir, spec.Captures, repoByID, roadmapByName, *fetch)
	must(err)

	out := manifest{
		SchemaVersion: 1,
		RoadmapTotal:  len(roadmap),
		Repositories:  repositories,
		Captures:      captures,
	}
	must(writeJSON(filepath.Join(corpusDir, "manifest.json"), out))
	must(writeReports(corpusDir, roadmap, captures))

	fmt.Printf("generated %d captures, %d packets, %d roadmap items\n", len(captures), totalPackets(captures), len(roadmap))
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repository root containing go.mod")
		}
		dir = parent
	}
}

func readJSON(fileName string, dst any) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return fmt.Errorf("read trailing JSON: %w", err)
	}
	return nil
}

func validateSpec(spec sourceSpec) error {
	if spec.Schema != "./source-spec.schema.json" {
		return fmt.Errorf("unexpected source schema %q", spec.Schema)
	}
	if spec.SchemaVersion != 1 {
		return fmt.Errorf("unsupported source schema version %d", spec.SchemaVersion)
	}
	if len(spec.Repositories) == 0 || len(spec.Captures) == 0 {
		return errors.New("source specification must include repositories and captures")
	}
	repos := make(map[string]repositorySpec, len(spec.Repositories))
	for _, repo := range spec.Repositories {
		origin := repo.Origin
		if origin == "" {
			origin = "github"
		}
		if !sourceIDPattern.MatchString(repo.ID) || !repositoryPattern.MatchString(repo.Repository) || !commitPattern.MatchString(repo.Commit) || repo.License == "" || !validProtocolCorpusUpstreamPath(repo.LicensePath) || !sha256Pattern.MatchString(repo.LicenseSHA256) {
			return fmt.Errorf("invalid repository specification: %+v", repo)
		}
		if origin != "github" && origin != "generated" {
			return fmt.Errorf("repository %q has invalid origin %q", repo.ID, origin)
		}
		if origin == "generated" && !validProtocolCorpusUpstreamPath(repo.Recipe) {
			return fmt.Errorf("generated repository %q is missing recipe", repo.ID)
		}
		if _, exists := repos[repo.ID]; exists {
			return fmt.Errorf("duplicate repository id %q", repo.ID)
		}
		repos[repo.ID] = repo
	}
	ids := make(map[string]struct{}, len(spec.Captures))
	for _, capture := range spec.Captures {
		if !sourceIDPattern.MatchString(capture.ID) {
			return fmt.Errorf("capture has invalid id %q", capture.ID)
		}
		if _, exists := ids[capture.ID]; exists {
			return fmt.Errorf("duplicate capture id %q", capture.ID)
		}
		ids[capture.ID] = struct{}{}
		if _, exists := repos[capture.RepositoryID]; !exists {
			return fmt.Errorf("capture %q references unknown repository %q", capture.ID, capture.RepositoryID)
		}
		if !validProtocolCorpusUpstreamPath(capture.UpstreamPath) || capture.Protocol == "" || capture.DisplayFilter == "" || !sha256Pattern.MatchString(capture.SourceSHA256) {
			return fmt.Errorf("capture %q has an empty required field", capture.ID)
		}
		if capture.RoadmapName != nil && *capture.RoadmapName == "" {
			return fmt.Errorf("capture %q has an empty roadmap name", capture.ID)
		}
		switch capture.EvidenceKind {
		case "upstream-positive", "upstream-negative", "generated-positive", "generated-negative", "generated-identification":
		default:
			return fmt.Errorf("capture %q has invalid evidence kind %q", capture.ID, capture.EvidenceKind)
		}
		generated := repos[capture.RepositoryID].Origin == "generated"
		if generated != strings.HasPrefix(capture.EvidenceKind, "generated-") {
			return fmt.Errorf("capture %q evidence kind does not match repository origin", capture.ID)
		}
		seenMappings := make(map[string]bool, len(capture.DecodeAs))
		for _, mapping := range capture.DecodeAs {
			if !decodeAsPattern.MatchString(mapping) || seenMappings[mapping] {
				return fmt.Errorf("capture %q has invalid or duplicate decode_as mapping %q", capture.ID, mapping)
			}
			seenMappings[mapping] = true
		}
	}
	return nil
}

func validProtocolCorpusUpstreamPath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	return clean == value && clean != ".." && !strings.HasPrefix(clean, "../")
}

func readRoadmap(fileName string) ([]roadmapItem, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []roadmapItem
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		match := roadmapPattern.FindStringSubmatch(scanner.Text())
		if len(match) != 5 {
			continue
		}
		items = append(items, roadmapItem{
			Name: match[1], Family: match[2],
			Status:   strings.ToLower(strings.TrimPrefix(match[3], "st")),
			Priority: strings.ToUpper(strings.TrimPrefix(match[4], "pri")),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(items) < 600 {
		return nil, fmt.Errorf("parsed only %d roadmap items; expected at least 600", len(items))
	}
	return items, nil
}

func prepareRepositories(corpusDir string, specs []repositorySpec, fetch bool) ([]manifestRepository, map[string]repositorySpec, error) {
	licenseDir := filepath.Join(corpusDir, "licenses")
	if err := os.MkdirAll(licenseDir, 0o755); err != nil {
		return nil, nil, err
	}
	result := make([]manifestRepository, 0, len(specs))
	byID := make(map[string]repositorySpec, len(specs))
	for _, repo := range specs {
		byID[repo.ID] = repo
		base := filepath.Base(repo.LicensePath)
		if !strings.HasSuffix(strings.ToLower(base), ".txt") {
			base += ".txt"
		}
		licenseName := repo.ID + "-" + base
		licenseAbs := filepath.Join(licenseDir, licenseName)
		if repo.Origin == "generated" {
			recipe, err := os.ReadFile(filepath.Join(corpusDir, repo.Recipe))
			if err != nil {
				return nil, nil, fmt.Errorf("read %s recipe: %w", repo.ID, err)
			}
			if digest := fmt.Sprintf("%x", sha1.Sum(recipe)); digest != repo.Commit {
				return nil, nil, fmt.Errorf("recipe digest mismatch for %s: got %s, want %s", repo.ID, digest, repo.Commit)
			}
			src := repo.LicensePath
			if !filepath.IsAbs(src) {
				src = filepath.Join(corpusDir, src)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return nil, nil, fmt.Errorf("read %s license: %w", repo.ID, err)
			}
			if err := os.WriteFile(licenseAbs, data, 0o644); err != nil {
				return nil, nil, err
			}
		} else {
			url := rawGitHubURL(repo, repo.LicensePath)
			if fetch {
				if err := download(url, licenseAbs); err != nil {
					return nil, nil, fmt.Errorf("download %s license: %w", repo.ID, err)
				}
			}
		}
		if _, err := os.Stat(licenseAbs); err != nil {
			return nil, nil, fmt.Errorf("missing %s; rerun with -fetch: %w", licenseAbs, err)
		}
		digest, _, err := fileDigest(licenseAbs)
		if err != nil {
			return nil, nil, err
		}
		if digest != repo.LicenseSHA256 {
			return nil, nil, fmt.Errorf("license digest mismatch for %s: got %s, want %s", repo.ID, digest, repo.LicenseSHA256)
		}
		result = append(result, manifestRepository{
			ID: repo.ID, Repository: repo.Repository, Commit: repo.Commit,
			License: repo.License, Homepage: repo.Homepage,
			LicenseFile: filepath.ToSlash(filepath.Join("licenses", licenseName)), LicenseSHA256: digest,
		})
	}
	return result, byID, nil
}

func prepareCaptures(corpusDir string, specs []captureSpec, repoByID map[string]repositorySpec, roadmap map[string]roadmapItem, fetch bool) ([]manifestCapture, error) {
	captureRoot := filepath.Join(corpusDir, "captures")
	if err := os.MkdirAll(captureRoot, 0o755); err != nil {
		return nil, err
	}

	result := make([]manifestCapture, 0, len(specs))
	for _, item := range specs {
		repo := repoByID[item.RepositoryID]
		ext := filepath.Ext(item.UpstreamPath)
		dir := filepath.Join(captureRoot, repo.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		captureName := item.ID + ext
		captureAbs := filepath.Join(dir, captureName)
		var sourceURL string
		if repo.Origin == "generated" {
			sourceURL = fmt.Sprintf("generated://scapy/%s/%s", repo.Commit, item.ID)
		} else {
			sourceURL = rawGitHubURL(repo, item.UpstreamPath)
			if fetch {
				if err := download(sourceURL, captureAbs); err != nil {
					return nil, fmt.Errorf("download %s: %w", item.ID, err)
				}
			}
		}
		if _, err := os.Stat(captureAbs); err != nil {
			return nil, fmt.Errorf("missing %s; rerun with -fetch: %w", captureAbs, err)
		}

		digest, size, err := fileDigest(captureAbs)
		if err != nil {
			return nil, err
		}
		if digest != item.SourceSHA256 {
			return nil, fmt.Errorf("capture digest mismatch for %s: got %s, want %s", item.ID, digest, item.SourceSHA256)
		}
		frameNumber, protocols, err := selectRepresentativeFrame(captureAbs, item.DisplayFilter, item.DecodeAs...)
		if err != nil {
			return nil, fmt.Errorf("select representative frame for %s: %w", item.ID, err)
		}
		count, linkType, frameBytes, err := inspectCapture(captureAbs, frameNumber)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", item.ID, err)
		}
		if count == 0 && !item.AllowEmpty {
			return nil, fmt.Errorf("capture %s is empty but allow_empty is false", item.ID)
		}
		if count > 0 && frameNumber == 0 {
			return nil, fmt.Errorf("display filter %q matched no frame in %s", item.DisplayFilter, item.ID)
		}

		var family, priority, status *string
		if item.RoadmapName != nil {
			r, exists := roadmap[*item.RoadmapName]
			if !exists {
				return nil, fmt.Errorf("capture %s maps to missing roadmap item %q", item.ID, *item.RoadmapName)
			}
			family, priority, status = ptr(r.Family), ptr(r.Priority), ptr(r.Status)
		}

		var rep *representativeFrame
		if frameNumber > 0 {
			frameDigest := sha256.Sum256(frameBytes)
			rep = &representativeFrame{
				Number: frameNumber, LengthBytes: len(frameBytes),
				SHA256: fmt.Sprintf("%x", frameDigest),
			}
		}

		result = append(result, manifestCapture{
			ID: item.ID, RepositoryID: item.RepositoryID, Protocol: item.Protocol,
			RoadmapName: item.RoadmapName, RoadmapFamily: family, RoadmapPriority: priority, RoadmapStatus: status,
			CaptureFile: filepath.ToSlash(filepath.Join("captures", repo.ID, captureName)),
			SourceURL:   sourceURL, UpstreamPath: item.UpstreamPath, SHA256: digest, SizeBytes: size,
			PacketCount: count, LinkType: linkType, DisplayFilter: item.DisplayFilter, DecodeAs: item.DecodeAs,
			EvidenceKind: item.EvidenceKind, Notes: item.Notes,
			RepresentativeFrame: rep, FrameProtocols: protocols,
		})
	}
	return result, nil
}

func rawGitHubURL(repo repositorySpec, filePath string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo.Repository, repo.Commit, filePath)
}

func download(url, destination string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return err
	}
	ok = true
	return nil
}

func fileDigest(fileName string) (string, int64, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func selectRepresentativeFrame(fileName, displayFilter string, decodeAs ...string) (int, []string, error) {
	args := []string{"-r", fileName, "-Y", displayFilter, "-T", "fields", "-e", "frame.number", "-e", "frame.protocols"}
	for _, mapping := range decodeAs {
		args = append(args, "-d", mapping)
	}
	cmd := exec.Command("tshark", args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return 0, nil, fmt.Errorf("tshark: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return 0, nil, fmt.Errorf("tshark: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 2)
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		number, err := strconv.Atoi(fields[0])
		if err != nil {
			return 0, nil, err
		}
		var protocols []string
		if len(fields) == 2 && fields[1] != "" {
			protocols = strings.Split(fields[1], ":")
		}
		return number, protocols, nil
	}
	return 0, []string{}, scanner.Err()
}

func inspectCapture(fileName string, representative int) (int, string, []byte, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return 0, "", nil, err
	}
	defer f.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(f, header); err != nil {
		return 0, "", nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, "", nil, err
	}

	var reader packetDataReader
	var linkType layers.LinkType
	if string(header) == "\x0a\x0d\x0d\x0a" {
		r, err := pcapgo.NewNgReader(f, pcapgo.NgReaderOptions{SkipUnknownVersion: true})
		if err != nil {
			return 0, "", nil, err
		}
		reader, linkType = r, r.LinkType()
	} else {
		r, err := pcapgo.NewReader(f)
		if err != nil {
			if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
				return 0, "", nil, err
			}
			data, readErr := io.ReadAll(f)
			if readErr != nil {
				return 0, "", nil, readErr
			}
			normalized, ok := normalizeLegacyPcapHeader(data)
			if !ok {
				return 0, "", nil, err
			}
			r, err = pcapgo.NewReader(bytes.NewReader(normalized))
			if err != nil {
				return 0, "", nil, err
			}
		}
		reader, linkType = r, r.LinkType()
	}

	count := 0
	var selected []byte
	for {
		data, _, err := reader.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, "", nil, err
		}
		count++
		if count == representative {
			selected = append([]byte(nil), data...)
		}
	}
	if representative > count {
		return 0, "", nil, fmt.Errorf("representative frame %d exceeds packet count %d", representative, count)
	}
	return count, corpusutil.LinkTypeName(linkType), selected, nil
}

// Some authoritative regression captures use the early pcap 2.1 header.
// Packet records have the same layout used by 2.4, so normalize a copy of the
// four version bytes for pcapgo while retaining and hashing the upstream file.
func normalizeLegacyPcapHeader(data []byte) ([]byte, bool) {
	if len(data) < 24 {
		return nil, false
	}
	bigEndian := bytes.Equal(data[:4], []byte{0xa1, 0xb2, 0xc3, 0xd4}) || bytes.Equal(data[:4], []byte{0xa1, 0xb2, 0x3c, 0x4d})
	littleEndian := bytes.Equal(data[:4], []byte{0xd4, 0xc3, 0xb2, 0xa1}) || bytes.Equal(data[:4], []byte{0x4d, 0x3c, 0xb2, 0xa1})
	normalized := append([]byte(nil), data...)
	switch {
	case bigEndian && data[4] == 0 && data[5] == 2 && data[6] == 0 && data[7] < 4:
		normalized[7] = 4
	case littleEndian && data[4] == 2 && data[5] == 0 && data[6] < 4 && data[7] == 0:
		normalized[6] = 4
	default:
		return nil, false
	}
	return normalized, true
}

func writeJSON(fileName string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(fileName, data, 0o644)
}

func writeReports(corpusDir string, roadmap []roadmapItem, captures []manifestCapture) error {
	reportDir := filepath.Join(corpusDir, "reports")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return err
	}
	counts := make(map[string]int)
	for _, capture := range captures {
		if capture.RoadmapName != nil {
			counts[*capture.RoadmapName]++
		}
	}
	return writeCoverageCSV(filepath.Join(reportDir, "roadmap-material-coverage.csv"), roadmap, counts)
}

func writeCoverageCSV(fileName string, roadmap []roadmapItem, counts map[string]int) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"protocol", "family", "priority", "implementation_status", "authoritative_capture_count", "material_status"}); err != nil {
		return err
	}
	for _, item := range roadmap {
		count := counts[item.Name]
		materialStatus := "missing"
		if count > 0 {
			materialStatus = "collected"
		}
		if err := w.Write([]string{item.Name, item.Family, item.Priority, item.Status, strconv.Itoa(count), materialStatus}); err != nil {
			return err
		}
	}
	return w.Error()
}

func totalPackets(captures []manifestCapture) int {
	total := 0
	for _, capture := range captures {
		total += capture.PacketCount
	}
	return total
}

func ptr(value string) *string { return &value }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "protocol corpus generator:", err)
		os.Exit(1)
	}
}
