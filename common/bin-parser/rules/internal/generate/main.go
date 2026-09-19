// Command generate produces the deterministic compressed rule archive.
package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var data bytes.Buffer
	tw := tar.NewWriter(&data)
	err := filepath.WalkDir(".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(name, ".yaml") {
			return nil
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		// Match repository LF semantics even on an existing autocrlf checkout.
		content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
		if err = tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Mode: 0444, Size: int64(len(content))}); err != nil {
			return err
		}
		_, err = tw.Write(content)
		return err
	})
	if err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithEncoderConcurrency(1))
	if err != nil {
		return err
	}
	packed := encoder.EncodeAll(data.Bytes(), nil)
	if err = encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile("rules.tar.zst", packed, 0644)
}
