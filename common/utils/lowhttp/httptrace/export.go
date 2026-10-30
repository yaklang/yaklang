package httptrace

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Export copies only the selected session/runtimes. An in-flight capture is a
// byte-length snapshot, with ended_at absent; it must not be called complete.
// Metadata contains no filesystem paths and cannot choose arbitrary zip inputs.
func Export(ctx context.Context, archive *zip.Writer, root, session string, runtimes []string) (int, error) {
	if session == "" && len(runtimes) == 0 {
		return 0, nil
	}
	if root == "" {
		root = DefaultDirectory()
	}
	search := root
	if session != "" {
		search = filepath.Join(root, bucket(Scope{SessionID: session}))
	}
	count := 0
	err := filepath.WalkDir(search, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "metadata.json" || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var meta Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return err
		}
		matches := session != "" && meta.SessionID == session
		if session == "" {
			for _, id := range runtimes {
				if meta.RuntimeID == id {
					matches = true
				}
			}
		}
		if !matches {
			return nil
		}
		// UUID directory names are generated locally, not from session text.
		prefix := "network/" + filepath.Base(filepath.Dir(path)) + "/"
		for _, name := range []string{"request.http", "response.http", "metadata.json"} {
			filePath := filepath.Join(filepath.Dir(path), name)
			stat, err := os.Lstat(filePath)
			if errors.Is(err, os.ErrNotExist) {
				continue
			} // capture_error describes missing files
			if err != nil {
				return err
			}
			if !stat.Mode().IsRegular() {
				continue
			}
			w, err := archive.Create(prefix + name)
			if err != nil {
				return err
			}
			if name == "metadata.json" {
				_, err = w.Write(data)
				if err != nil {
					return err
				}
				continue
			}
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(w, file, stat.Size())
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		count++
		return nil
	})
	return count, err
}
