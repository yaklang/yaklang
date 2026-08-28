package scannode

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

const (
	runtimeHostArchiveManifestLimit = 1 << 20
	runtimeHostArchiveConfigLimit   = 4 << 20
)

var runtimeHostImageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var runtimeHostImageTagPattern = regexp.MustCompile(`^local\.invalid/yaklang/legion-ai-session-runtime:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

type runtimeHostImageRecord struct {
	ReleaseID       string    `json:"release_id"`
	EngineDigest    string    `json:"engine_digest"`
	ProducerImageID string    `json:"producer_image_id"`
	ArchiveSHA256   string    `json:"archive_sha256"`
	ArchiveSize     int64     `json:"archive_size"`
	ImageTag        string    `json:"image_tag"`
	LocalImageID    string    `json:"local_image_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type runtimeHostArchiveManifestEntry struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
}

func (e *runtimeHostExecutor) resolveRuntimeImage(ctx context.Context, release *nodev1.AIRuntimeRelease) (string, bool, error) {
	producerImageID := strings.ToLower(strings.TrimSpace(release.GetImageId()))
	imageID, exists, err := e.docker.ResolveImageID(ctx, producerImageID)
	if err != nil {
		return "", false, err
	}
	if exists {
		imageID = strings.ToLower(strings.TrimSpace(imageID))
		if !runtimeHostImageIDPattern.MatchString(imageID) {
			return "", false, fmt.Errorf("Runtime image has an invalid target-local identity")
		}
		return imageID, true, nil
	}

	record, ok := e.images[release.GetReleaseId()]
	if !ok || !runtimeHostImageRecordMatchesRelease(record, release) {
		return "", false, nil
	}
	imageID, exists, err = e.docker.ResolveImageID(ctx, record.ImageTag)
	if err != nil || !exists {
		return "", exists, err
	}
	imageID = strings.ToLower(strings.TrimSpace(imageID))
	if imageID != record.LocalImageID {
		// Treat a changed or retagged local descriptor as not ready. ENSURE_IMAGE
		// will re-validate and re-load the exact pinned archive before replacing
		// the durable mapping; START never adopts the changed image directly.
		return "", false, nil
	}
	return imageID, true, nil
}

func runtimeHostImageRecordMatchesRelease(record runtimeHostImageRecord, release *nodev1.AIRuntimeRelease) bool {
	return record.ReleaseID == release.GetReleaseId() &&
		strings.EqualFold(record.EngineDigest, release.GetEngineDigest()) &&
		strings.EqualFold(record.ProducerImageID, release.GetImageId()) &&
		strings.EqualFold(record.ArchiveSHA256, release.GetArchiveSha256()) &&
		record.ArchiveSize == release.GetArchiveSize() &&
		runtimeHostImageTagPattern.MatchString(record.ImageTag) &&
		runtimeHostImageIDPattern.MatchString(record.LocalImageID)
}

func (e *runtimeHostExecutor) recordRuntimeImage(release *nodev1.AIRuntimeRelease, imageTag, localImageID string) error {
	record := runtimeHostImageRecord{
		ReleaseID:       release.GetReleaseId(),
		EngineDigest:    strings.ToLower(release.GetEngineDigest()),
		ProducerImageID: strings.ToLower(release.GetImageId()),
		ArchiveSHA256:   strings.ToLower(release.GetArchiveSha256()),
		ArchiveSize:     release.GetArchiveSize(),
		ImageTag:        imageTag,
		LocalImageID:    strings.ToLower(strings.TrimSpace(localImageID)),
		UpdatedAt:       time.Now().UTC(),
	}
	previous, existed := e.images[record.ReleaseID]
	e.images[record.ReleaseID] = record
	if err := e.saveOperationJournal(); err != nil {
		if existed {
			e.images[record.ReleaseID] = previous
		} else {
			delete(e.images, record.ReleaseID)
		}
		return err
	}
	return nil
}

func inspectRuntimeImageArchive(archive io.ReadSeeker, release *nodev1.AIRuntimeRelease) (string, error) {
	producerImageID := strings.ToLower(strings.TrimSpace(release.GetImageId()))
	if !runtimeHostImageIDPattern.MatchString(producerImageID) {
		return "", fmt.Errorf("Runtime archive producer image identity is invalid")
	}
	producerDigest := strings.TrimPrefix(producerImageID, "sha256:")
	manifestJSON, configDigests, err := inspectRuntimeArchiveEntries(archive, producerDigest)
	if err != nil {
		return "", fmt.Errorf("inspect Runtime archive: %w", err)
	}
	manifest := make([]runtimeHostArchiveManifestEntry, 0, 1)
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return "", fmt.Errorf("decode Runtime archive manifest: %w", err)
	}
	if len(manifest) != 1 || len(manifest[0].RepoTags) != 1 {
		return "", fmt.Errorf("Runtime archive must contain exactly one tagged image")
	}
	imageTag := strings.TrimSpace(manifest[0].RepoTags[0])
	if !runtimeHostImageTagPattern.MatchString(imageTag) {
		return "", fmt.Errorf("Runtime archive image tag is not the fixed local release tag")
	}
	configPath := strings.TrimSpace(manifest[0].Config)
	if configPath != producerDigest+".json" && configPath != "blobs/sha256/"+producerDigest {
		return "", fmt.Errorf("Runtime archive config path does not match the producer image identity")
	}
	if configDigests[configPath] != producerDigest {
		return "", fmt.Errorf("Runtime archive image config does not match the producer image identity")
	}
	return imageTag, nil
}

func inspectRuntimeArchiveEntries(archive io.ReadSeeker, producerDigest string) ([]byte, map[string]string, error) {
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return nil, nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var manifestJSON []byte
	configDigests := make(map[string]string, 2)
	legacyConfigPath := producerDigest + ".json"
	contentStoreConfigPath := "blobs/sha256/" + producerDigest
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		name := header.Name
		if name != "manifest.json" && name != legacyConfigPath && name != contentStoreConfigPath {
			continue
		}
		if name == "manifest.json" && manifestJSON != nil {
			return nil, nil, fmt.Errorf("archive contains duplicate manifest.json")
		}
		if _, duplicate := configDigests[name]; duplicate {
			return nil, nil, fmt.Errorf("archive contains duplicate %s", name)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, nil, fmt.Errorf("archive entry %s is not a regular file", name)
		}
		sizeLimit := int64(runtimeHostArchiveConfigLimit)
		if name == "manifest.json" {
			sizeLimit = runtimeHostArchiveManifestLimit
		}
		if header.Size < 0 || header.Size > sizeLimit {
			return nil, nil, fmt.Errorf("archive entry %s exceeds the size limit", name)
		}
		data, err := io.ReadAll(io.LimitReader(tarReader, sizeLimit+1))
		if err != nil {
			return nil, nil, err
		}
		if int64(len(data)) != header.Size {
			return nil, nil, fmt.Errorf("archive entry %s is truncated", name)
		}
		if name == "manifest.json" {
			manifestJSON = data
			continue
		}
		digest := sha256.Sum256(data)
		configDigests[name] = hex.EncodeToString(digest[:])
	}
	if manifestJSON == nil {
		return nil, nil, fmt.Errorf("archive does not contain manifest.json")
	}
	return manifestJSON, configDigests, nil
}
