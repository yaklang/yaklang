// Package inputresolver materializes managed inputs before Agent execution.
// Resource authorization belongs to the caller; manifests contain only pinned
// identities and never grant access by themselves.
package inputresolver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"path"
	"regexp"
	"strings"
	"unicode"

	"google.golang.org/protobuf/proto"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	SchemaV1             = "legion.input-manifest/v1"
	ManagedAttachment    = "managed_attachment"
	CapabilityV1         = "ai.input.managed_attachment.v1"
	RuntimeKey           = "input_manifest"
	ManifestIDRuntimeKey = "input_manifest_id"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$`)
var safeField = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Digest is stable across protobuf implementations and excludes only its own
// digest field. Resource order is significant and follows the input contract.
func Digest(manifest *aiv1.InputManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("input manifest is missing")
	}
	copy := proto.Clone(manifest).(*aiv1.InputManifest)
	copy.ManifestId = ""
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(copy)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func Seal(manifest *aiv1.InputManifest) error {
	digest, err := Digest(manifest)
	if err != nil {
		return err
	}
	manifest.ManifestId = digest
	return Validate(manifest)
}

// Validate rejects unsupported policy instead of inferring defaults that a
// different runtime might interpret less strictly.
func Validate(manifest *aiv1.InputManifest) error {
	if manifest == nil || manifest.SchemaVersion != SchemaV1 {
		return fmt.Errorf("unsupported input manifest version")
	}
	for _, id := range []string{manifest.OwnerUserId, manifest.ProductKey, manifest.RunId,
		manifest.SessionId, manifest.AttemptId, manifest.WorkspaceId, manifest.AttemptCommandId} {
		if !safeID.MatchString(id) {
			return fmt.Errorf("invalid input manifest execution identity")
		}
	}
	if manifest.OutputPath != "outputs" || len(manifest.Resources) == 0 || len(manifest.Resources) > 128 {
		return fmt.Errorf("invalid input manifest workspace policy")
	}
	seenIDs, seenPaths := map[string]bool{}, map[string]bool{}
	var total uint64
	for _, resource := range manifest.Resources {
		if resource == nil || resource.Kind != ManagedAttachment || !safeID.MatchString(resource.ResourceId) ||
			!safeField.MatchString(resource.InputField) || !resource.Required || !resource.ReadOnly {
			return fmt.Errorf("unsupported input resource or policy")
		}
		if !SafeInputPath(resource.RelativePath) || resource.Filename == "" ||
			resource.SizeBytes > math.MaxInt64 || resource.SizeBytes > math.MaxInt64-total {
			return fmt.Errorf("invalid input resource path or size")
		}
		digest, err := hex.DecodeString(resource.Sha256)
		if err != nil || len(digest) != sha256.Size || strings.ToLower(resource.Sha256) != resource.Sha256 {
			return fmt.Errorf("invalid input resource digest")
		}
		if seenIDs[resource.ResourceId] || seenPaths[strings.ToLower(resource.RelativePath)] {
			return fmt.Errorf("duplicate input resource or path")
		}
		seenIDs[resource.ResourceId], seenPaths[strings.ToLower(resource.RelativePath)] = true, true
		total += resource.SizeBytes
	}
	digest, err := Digest(manifest)
	if err != nil || manifest.ManifestId != digest {
		return fmt.Errorf("input manifest integrity mismatch")
	}
	return nil
}

func SafeInputPath(value string) bool {
	if len(value) > 512 || !strings.HasPrefix(value, "inputs/") || path.Clean(value) != value ||
		strings.ContainsAny(value, "\\:\x00") || strings.HasSuffix(value, "/") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}

// RelativePath assigns a unique logical path without trusting the uploaded
// filename as a path. The original display filename stays in the manifest.
func RelativePath(field string, index int, filename string) string {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" || len(name) > 180 {
		name = "attachment"
	}
	return fmt.Sprintf("inputs/%s/%03d-%s", field, index+1, name)
}
