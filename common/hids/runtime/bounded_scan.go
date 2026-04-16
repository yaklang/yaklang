//go:build hids && linux

package runtime

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	defaultDirectoryScanMaxEntries = 16
	maxDirectoryScanMaxEntries     = 128
	defaultDirectoryScanMaxDepth   = 1
	maxDirectoryScanMaxDepth       = 6
)

var errDirectoryScanTruncated = errors.New("directory scan truncated")

type boundedDirectoryScanOptions struct {
	MaxEntries int
	MaxDepth   int
	Recursive  bool
}

type boundedDirectoryScanResult struct {
	RootArtifact   *model.Artifact
	Entries        []boundedScanEntry
	ScannedCount   int
	FileCount      int
	DirectoryCount int
	Truncated      bool
	MaxEntries     int
	MaxDepth       int
	Recursive      bool
}

type boundedScanEntry struct {
	Path         string
	RelativePath string
	Depth        int
	IsDir        bool
	Artifact     *model.Artifact
}

func normalizeDirectoryScanOptions(request map[string]any) boundedDirectoryScanOptions {
	metadata := readEvidenceMetadata(request)
	options := boundedDirectoryScanOptions{
		MaxEntries: defaultDirectoryScanMaxEntries,
		MaxDepth:   defaultDirectoryScanMaxDepth,
	}

	recursive := anyToBool(firstNonNilEvidence(metadata["recursive"], request["recursive"]))
	if recursive {
		options.Recursive = true
		options.MaxDepth = 2
	}

	if maxEntries, ok := anyToInt(firstNonNilEvidence(metadata["max_entries"], request["max_entries"])); ok && maxEntries > 0 {
		options.MaxEntries = maxEntries
	}
	if maxDepth, ok := anyToInt(firstNonNilEvidence(metadata["max_depth"], request["max_depth"])); ok && maxDepth >= 0 {
		options.MaxDepth = maxDepth
	}

	if options.MaxEntries <= 0 {
		options.MaxEntries = defaultDirectoryScanMaxEntries
	}
	if options.MaxEntries > maxDirectoryScanMaxEntries {
		options.MaxEntries = maxDirectoryScanMaxEntries
	}
	if options.MaxDepth < 0 {
		options.MaxDepth = 0
	}
	if options.MaxDepth > maxDirectoryScanMaxDepth {
		options.MaxDepth = maxDirectoryScanMaxDepth
	}
	if options.MaxDepth > defaultDirectoryScanMaxDepth {
		options.Recursive = true
	}
	return options
}

func (p *pipeline) scanDirectory(path string, options boundedDirectoryScanOptions) (boundedDirectoryScanResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return boundedDirectoryScanResult{}, fmt.Errorf("directory path is required")
	}

	rootArtifact, err := p.snapshotEvidenceArtifact(path)
	if err != nil && rootArtifact == nil {
		return boundedDirectoryScanResult{}, err
	}
	if rootArtifact == nil || !rootArtifact.Exists {
		return boundedDirectoryScanResult{}, fmt.Errorf("directory %q does not exist", path)
	}
	if rootArtifact.FileType != "directory" {
		return boundedDirectoryScanResult{}, fmt.Errorf("path %q is not a directory", path)
	}

	result := boundedDirectoryScanResult{
		RootArtifact: rootArtifact,
		MaxEntries:   options.MaxEntries,
		MaxDepth:     options.MaxDepth,
		Recursive:    options.Recursive,
	}

	walkErr := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relativePath, relativeErr := filepath.Rel(path, current)
		if relativeErr != nil {
			return relativeErr
		}
		if relativePath == "." {
			return nil
		}

		depth := scanPathDepth(relativePath)
		if depth > options.MaxDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if result.ScannedCount >= options.MaxEntries {
			result.Truncated = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return errDirectoryScanTruncated
		}

		artifact, snapshotErr := p.snapshotEvidenceArtifact(current)
		if snapshotErr != nil && artifact == nil {
			return snapshotErr
		}

		result.ScannedCount++
		if entry.IsDir() {
			result.DirectoryCount++
		} else {
			result.FileCount++
		}
		result.Entries = append(result.Entries, boundedScanEntry{
			Path:         current,
			RelativePath: filepath.ToSlash(relativePath),
			Depth:        depth,
			IsDir:        entry.IsDir(),
			Artifact:     artifact,
		})

		if result.ScannedCount >= options.MaxEntries {
			result.Truncated = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return errDirectoryScanTruncated
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errDirectoryScanTruncated) {
		return result, walkErr
	}

	sort.Slice(result.Entries, func(left, right int) bool {
		return result.Entries[left].RelativePath < result.Entries[right].RelativePath
	})
	return result, nil
}

func scanPathDepth(relativePath string) int {
	relativePath = filepath.ToSlash(strings.TrimSpace(relativePath))
	if relativePath == "" || relativePath == "." {
		return 0
	}
	return strings.Count(relativePath, "/") + 1
}
