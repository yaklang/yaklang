package builtinmetadata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Scan reads every resource once. Path, count, permissions, size and content all
// participate; identical sizes/mtimes cannot hide an edit. Timestamps are omitted
// because a clean checkout changes them without changing the embedded resource.
func Scan(root string) (map[string]File, error) {
	files := map[string]File{}
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".tar.gz") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported resource type: %s", name)
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		item := File{Mode: uint32(info.Mode())}
		if !info.IsDir() {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			item.Size = int64(len(data))
			item.SHA256 = Digest(data)
		}
		files[filepath.ToSlash(relative)] = item
		return nil
	})
	return files, err
}

// ValidateArchive checks the complete archive, including additions, removals,
// renames, empty directories and permissions. It never invokes a parser.
func (b *Bundle) ValidateArchive(data []byte, prefix string) error {
	decoded, err := gzip_embed.DecodeArchive(bytes.Clone(data))
	if err != nil {
		return err
	}
	r, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	seen := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return err
			}
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		if prefix != "" {
			if name == prefix {
				name = "."
			} else {
				name = strings.TrimPrefix(name, prefix+"/")
			}
		}
		if !fs.ValidPath(name) || path.Clean(name) != name || seen[name] {
			return fmt.Errorf("invalid resource path: %s", name)
		}
		file, ok := b.Files[name]
		if !ok {
			return fmt.Errorf("resource missing from metadata: %s", name)
		}
		if uint32(h.FileInfo().Mode()) != file.Mode || (!h.FileInfo().IsDir() && h.Size != file.Size) {
			return fmt.Errorf("resource attributes changed: %s", name)
		}
		if !h.FileInfo().IsDir() {
			hash := shaReader(tr)
			if hash != file.SHA256 {
				return fmt.Errorf("resource content changed: %s", name)
			}
		}
		seen[name] = true
	}
	for name := range b.Files {
		if name != "." && !seen[name] {
			return fmt.Errorf("metadata resource missing from archive: %s", name)
		}
	}
	return nil
}

func shaReader(r io.Reader) string {
	// Stream rule sets without materializing another copy of every source file.
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// LocalHash verifies that a generated metadata file describes this source set.
// Hash-writing callers must never overwrite a metadata hash with a source hash.
func LocalHash(sourceDir, metadataPath, kind string) (string, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", err
	}
	bundle, err := Parse(data, kind)
	if err != nil {
		return "", err
	}
	files, err := Scan(sourceDir)
	if err != nil {
		return "", err
	}
	if !bundle.Matches(files, bundle.Recipe) {
		return "", fmt.Errorf("%s metadata is stale; refresh embedded resource hashes", kind)
	}
	packed, err := Encode(bundle)
	if err != nil {
		return "", err
	}
	return Digest(packed), nil
}
