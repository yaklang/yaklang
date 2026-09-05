// Command generate downloads pinned upstream captures and builds the protocol
// corpus manifest, representative frame hex and coverage reports.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

type sourceSpec struct {
	Schema        string           `json:"$schema,omitempty"`
	SchemaVersion int              `json:"schema_version"`
	Repositories  []repositorySpec `json:"repositories"`
	Captures      []captureSpec    `json:"captures"`
}

type repositorySpec struct {
	ID          string `json:"id"`
	Repository  string `json:"repository"`
	Commit      string `json:"commit"`
	License     string `json:"license"`
	LicensePath string `json:"license_path"`
	Homepage    string `json:"homepage,omitempty"`
}

type captureSpec struct {
	ID            string  `json:"id"`
	RepositoryID  string  `json:"repository_id"`
	UpstreamPath  string  `json:"upstream_path"`
	Protocol      string  `json:"protocol"`
	RoadmapName   *string `json:"roadmap_name"`
	DisplayFilter string  `json:"display_filter"`
	EvidenceKind  string  `json:"evidence_kind"`
	Notes         string  `json:"notes,omitempty"`
	AllowEmpty    bool    `json:"allow_empty,omitempty"`
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
	EvidenceKind        string               `json:"evidence_kind"`
	Notes               string               `json:"notes,omitempty"`
	RepresentativeFrame *representativeFrame `json:"representative_frame"`
	FrameProtocols      []string             `json:"frame_protocols"`
}

type representativeFrame struct {
	Number      int    `json:"number"`
	LengthBytes int    `json:"length_bytes"`
	HexFile     string `json:"hex_file"`
}

type roadmapItem struct {
	Name     string
	Family   string
	Status   string
	Priority string
}

type familyStat struct {
	Family        string
	RoadmapTotal  int
	CoveredUnique int
	CaptureCount  int
}

type packetDataReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
}

var roadmapPattern = regexp.MustCompile(`\{Name: "([^"]+)", Family: "([^"]+)".*Status: (st\w+), Priority: (pri\w+)`)

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
	return dec.Decode(dst)
}

func validateSpec(spec sourceSpec) error {
	if spec.SchemaVersion != 1 {
		return fmt.Errorf("unsupported source schema version %d", spec.SchemaVersion)
	}
	repos := make(map[string]struct{}, len(spec.Repositories))
	for _, repo := range spec.Repositories {
		if repo.ID == "" || repo.Repository == "" || len(repo.Commit) != 40 || repo.LicensePath == "" {
			return fmt.Errorf("invalid repository specification: %+v", repo)
		}
		if _, exists := repos[repo.ID]; exists {
			return fmt.Errorf("duplicate repository id %q", repo.ID)
		}
		repos[repo.ID] = struct{}{}
	}
	ids := make(map[string]struct{}, len(spec.Captures))
	for _, capture := range spec.Captures {
		if _, exists := ids[capture.ID]; exists {
			return fmt.Errorf("duplicate capture id %q", capture.ID)
		}
		ids[capture.ID] = struct{}{}
		if _, exists := repos[capture.RepositoryID]; !exists {
			return fmt.Errorf("capture %q references unknown repository %q", capture.ID, capture.RepositoryID)
		}
		if capture.UpstreamPath == "" || capture.Protocol == "" || capture.DisplayFilter == "" {
			return fmt.Errorf("capture %q has an empty required field", capture.ID)
		}
		switch capture.EvidenceKind {
		case "upstream-positive", "upstream-negative", "educational-challenge":
		default:
			return fmt.Errorf("capture %q has invalid evidence kind %q", capture.ID, capture.EvidenceKind)
		}
	}
	return nil
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
		licenseName := repo.ID + "-" + filepath.Base(repo.LicensePath) + ".txt"
		licenseAbs := filepath.Join(licenseDir, licenseName)
		url := rawGitHubURL(repo, repo.LicensePath)
		if fetch {
			if err := download(url, licenseAbs); err != nil {
				return nil, nil, fmt.Errorf("download %s license: %w", repo.ID, err)
			}
		}
		if _, err := os.Stat(licenseAbs); err != nil {
			return nil, nil, fmt.Errorf("missing %s; rerun with -fetch: %w", licenseAbs, err)
		}
		digest, _, err := fileDigest(licenseAbs)
		if err != nil {
			return nil, nil, err
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
	hexDir := filepath.Join(corpusDir, "hex")
	if err := os.MkdirAll(captureRoot, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(hexDir, 0o755); err != nil {
		return nil, err
	}
	if err := removeGeneratedHex(hexDir); err != nil {
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
		sourceURL := rawGitHubURL(repo, item.UpstreamPath)
		if fetch {
			if err := download(sourceURL, captureAbs); err != nil {
				return nil, fmt.Errorf("download %s: %w", item.ID, err)
			}
		}
		if _, err := os.Stat(captureAbs); err != nil {
			return nil, fmt.Errorf("missing %s; rerun with -fetch: %w", captureAbs, err)
		}

		digest, size, err := fileDigest(captureAbs)
		if err != nil {
			return nil, err
		}
		frameNumber, protocols, err := selectRepresentativeFrame(captureAbs, item.DisplayFilter)
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
			hexName := fmt.Sprintf("%s.frame-%d.hex", item.ID, frameNumber)
			hexAbs := filepath.Join(hexDir, hexName)
			if err := os.WriteFile(hexAbs, []byte(hex.EncodeToString(frameBytes)+"\n"), 0o644); err != nil {
				return nil, err
			}
			rep = &representativeFrame{
				Number: frameNumber, LengthBytes: len(frameBytes),
				HexFile: filepath.ToSlash(filepath.Join("hex", hexName)),
			}
		}

		result = append(result, manifestCapture{
			ID: item.ID, RepositoryID: item.RepositoryID, Protocol: item.Protocol,
			RoadmapName: item.RoadmapName, RoadmapFamily: family, RoadmapPriority: priority, RoadmapStatus: status,
			CaptureFile: filepath.ToSlash(filepath.Join("captures", repo.ID, captureName)),
			SourceURL:   sourceURL, UpstreamPath: item.UpstreamPath, SHA256: digest, SizeBytes: size,
			PacketCount: count, LinkType: linkType, DisplayFilter: item.DisplayFilter,
			EvidenceKind: item.EvidenceKind, Notes: item.Notes,
			RepresentativeFrame: rep, FrameProtocols: protocols,
		})
	}
	return result, nil
}

func removeGeneratedHex(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".hex") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
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

func selectRepresentativeFrame(fileName, displayFilter string) (int, []string, error) {
	cmd := exec.Command("tshark", "-r", fileName, "-Y", displayFilter, "-T", "fields", "-e", "frame.number", "-e", "frame.protocols")
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
	return count, linkType.String(), selected, nil
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
	if err := writeCoverageCSV(filepath.Join(reportDir, "roadmap-material-coverage.csv"), roadmap, counts); err != nil {
		return err
	}
	if err := writeOutsideCSV(filepath.Join(reportDir, "outside-roadmap-candidates.csv"), captures); err != nil {
		return err
	}
	stats := familyStats(roadmap, captures)
	if err := writeDistributionCSV(filepath.Join(reportDir, "family-distribution.csv"), stats); err != nil {
		return err
	}
	if err := writeDistributionSVG(filepath.Join(reportDir, "protocol-material-distribution.svg"), stats); err != nil {
		return err
	}
	return writeReportMarkdown(filepath.Join(reportDir, "REPORT.md"), roadmap, captures, stats)
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

func writeOutsideCSV(fileName string, captures []manifestCapture) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"capture_id", "protocol", "source", "evidence_kind", "notes"}); err != nil {
		return err
	}
	for _, capture := range captures {
		if capture.RoadmapName == nil {
			if err := w.Write([]string{capture.ID, capture.Protocol, capture.RepositoryID, capture.EvidenceKind, capture.Notes}); err != nil {
				return err
			}
		}
	}
	return w.Error()
}

func familyStats(roadmap []roadmapItem, captures []manifestCapture) []familyStat {
	byFamily := make(map[string]*familyStat)
	for _, item := range roadmap {
		stat := byFamily[item.Family]
		if stat == nil {
			stat = &familyStat{Family: item.Family}
			byFamily[item.Family] = stat
		}
		stat.RoadmapTotal++
	}
	covered := make(map[string]struct{})
	for _, capture := range captures {
		if capture.RoadmapName == nil || capture.RoadmapFamily == nil {
			continue
		}
		stat := byFamily[*capture.RoadmapFamily]
		stat.CaptureCount++
		key := *capture.RoadmapFamily + "\x00" + *capture.RoadmapName
		if _, exists := covered[key]; !exists {
			covered[key] = struct{}{}
			stat.CoveredUnique++
		}
	}
	stats := make([]familyStat, 0, len(byFamily))
	for _, stat := range byFamily {
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].RoadmapTotal == stats[j].RoadmapTotal {
			return stats[i].Family < stats[j].Family
		}
		return stats[i].RoadmapTotal > stats[j].RoadmapTotal
	})
	return stats
}

func writeDistributionCSV(fileName string, stats []familyStat) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"family", "roadmap_protocols", "protocols_with_collected_capture", "capture_files"}); err != nil {
		return err
	}
	for _, stat := range stats {
		if err := w.Write([]string{stat.Family, strconv.Itoa(stat.RoadmapTotal), strconv.Itoa(stat.CoveredUnique), strconv.Itoa(stat.CaptureCount)}); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeDistributionSVG(fileName string, stats []familyStat) error {
	const width, labelWidth, barWidth, top, rowHeight = 1180, 180, 760, 116, 32
	height := top + len(stats)*rowHeight + 74
	maxTotal := 1
	for _, stat := range stats {
		if stat.RoadmapTotal > maxTotal {
			maxTotal = stat.RoadmapTotal
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-labelledby="title desc">`, width, height, width, height)
	b.WriteString(`<title id="title">Protocol roadmap and collected authoritative material by family</title>`)
	b.WriteString(`<desc id="desc">Horizontal bars compare all roadmap protocols with unique protocols that have at least one collected upstream or educational challenge capture. Exact covered and total counts are printed on every row.</desc>`)
	b.WriteString(`<rect width="100%" height="100%" fill="#FFFFFF"/>`)
	b.WriteString(`<style>text{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;fill:#171717}.title{font-size:22px;font-weight:650}.subtitle{font-size:13px;fill:#59636E}.label{font-size:13px}.count{font-size:12px;font-variant-numeric:tabular-nums}.legend{font-size:12px;fill:#59636E}</style>`)
	b.WriteString(`<text class="title" x="28" y="36">616-item protocol roadmap: collected capture material</text>`)
	b.WriteString(`<text class="subtitle" x="28" y="60">Blue is the unique protocol count with an authoritative PCAP; gray is the full roadmap family. Counts are capture availability, not parser correctness.</text>`)
	b.WriteString(`<rect x="28" y="76" width="14" height="10" fill="#0072B2"/><text class="legend" x="49" y="86">collected protocol</text>`)
	b.WriteString(`<rect x="190" y="76" width="14" height="10" fill="#D6DBE1"/><text class="legend" x="211" y="86">roadmap total</text>`)
	for index, stat := range stats {
		y := top + index*rowHeight
		totalWidth := stat.RoadmapTotal * barWidth / maxTotal
		coveredWidth := stat.CoveredUnique * barWidth / maxTotal
		fmt.Fprintf(&b, `<text class="label" x="%d" y="%d" text-anchor="end">%s</text>`, labelWidth-12, y+14, html.EscapeString(stat.Family))
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="17" fill="#D6DBE1"/>`, labelWidth, y, totalWidth)
		if coveredWidth > 0 {
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="17" fill="#0072B2"/>`, labelWidth, y, coveredWidth)
		}
		fmt.Fprintf(&b, `<text class="count" x="%d" y="%d">%d / %d protocols · %d captures</text>`, labelWidth+totalWidth+10, y+14, stat.CoveredUnique, stat.RoadmapTotal, stat.CaptureCount)
	}
	b.WriteString(`</svg>`)
	return os.WriteFile(fileName, []byte(b.String()), 0o644)
}

func writeReportMarkdown(fileName string, roadmap []roadmapItem, captures []manifestCapture, stats []familyStat) error {
	statusCounts := map[string]int{}
	priorityCounts := map[string]int{}
	sourceCaptures := map[string]int{}
	sourcePackets := map[string]int{}
	kindCounts := map[string]int{}
	covered := map[string]struct{}{}
	outside := 0
	var totalBytes int64
	for _, item := range roadmap {
		statusCounts[item.Status]++
		priorityCounts[item.Priority]++
	}
	for _, capture := range captures {
		sourceCaptures[capture.RepositoryID]++
		sourcePackets[capture.RepositoryID] += capture.PacketCount
		kindCounts[capture.EvidenceKind]++
		totalBytes += capture.SizeBytes
		if capture.RoadmapName == nil {
			outside++
		} else {
			covered[*capture.RoadmapName] = struct{}{}
		}
	}

	var b strings.Builder
	b.WriteString("# Protocol corpus evidence report\n\n")
	b.WriteString("This report is generated from `sources.json`, the pinned capture bytes and `protocol_roadmap.go`. It reports material availability only; it does not promote any roadmap status.\n\n")
	fmt.Fprintf(&b, "- Roadmap: **%d** protocols; %d `done`, %d `partial`, %d `todo`.\n", len(roadmap), statusCounts["done"], statusCounts["partial"], statusCounts["todo"])
	fmt.Fprintf(&b, "- Corpus: **%d capture files**, **%d packets**, **%d bytes**.\n", len(captures), totalPackets(captures), totalBytes)
	fmt.Fprintf(&b, "- Direct roadmap material: **%d unique protocols**; outside-roadmap candidates: **%d captures**.\n", len(covered), outside)
	fmt.Fprintf(&b, "- Evidence classes: %d positive upstream, %d negative/boundary upstream, %d official educational challenge.\n\n", kindCounts["upstream-positive"], kindCounts["upstream-negative"], kindCounts["educational-challenge"])

	b.WriteString("## Source distribution\n\n| Source | Captures | Packets |\n| --- | ---: | ---: |\n")
	sources := sortedKeys(sourceCaptures)
	for _, source := range sources {
		fmt.Fprintf(&b, "| `%s` | %d | %d |\n", source, sourceCaptures[source], sourcePackets[source])
	}
	b.WriteString("\n## Roadmap family distribution\n\n| Family | Roadmap protocols | With collected capture | Capture files |\n| --- | ---: | ---: | ---: |\n")
	for _, stat := range stats {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d |\n", stat.Family, stat.RoadmapTotal, stat.CoveredUnique, stat.CaptureCount)
	}
	b.WriteString("\n![Protocol material distribution. Every row prints collected protocol count over the full roadmap family count.](protocol-material-distribution.svg)\n\n")
	b.WriteString("**Figure 1 | Authoritative capture material by roadmap family.** Blue marks unique roadmap protocols with at least one collected file; the gray extent is the full family backlog. Exact values are printed, so color is not the only encoding.\n\n")
	b.WriteString("## Interpretation limits\n\n")
	b.WriteString("A capture mapped to a roadmap item establishes available test material, not complete protocol coverage. A single PCAP may exercise only one PDU, direction or version. Negative captures are kept separately because malformed input and false-positive resistance are part of parser authentication. `outside-roadmap-candidates.csv` records useful discoveries without pretending they were already among the 616 items.\n")
	return os.WriteFile(fileName, []byte(b.String()), 0o644)
}

func totalPackets(captures []manifestCapture) int {
	total := 0
	for _, capture := range captures {
		total += capture.PacketCount
	}
	return total
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func ptr(value string) *string { return &value }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "protocol corpus generator:", err)
		os.Exit(1)
	}
}
