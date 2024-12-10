package gzip_embed

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

// PreprocessingEmbed is a simple tools to read file from embed.FS and gzip compress file
// only support ReadFile method, not support Open method
type PreprocessingEmbed struct {
	*embed.FS
	EnableCache    bool
	cacheFile      map[string][]byte
	sourceFileName string
}

func NewEmptyPreprocessingEmbed() *PreprocessingEmbed {
	return &PreprocessingEmbed{
		cacheFile: map[string][]byte{},
	}
}

// NewPreprocessingEmbed create a CompressFS instance
// fs is embed.FS instance, compressDirs is a map, key is virtual dir, value is compress file name
func NewPreprocessingEmbed(fs *embed.FS, fileName string, cache bool) (*PreprocessingEmbed, error) {
	cfs := &PreprocessingEmbed{
		FS:             fs,
		cacheFile:      map[string][]byte{},
		sourceFileName: fileName,
		EnableCache:    cache,
	}
	if cache {
		err := cfs.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
			buf := &bytes.Buffer{}
			if _, err := io.Copy(buf, reader); err != nil {
				return err, true
			}
			cfs.cacheFile[header.Name] = buf.Bytes()
			return nil, true
		})
		if err != nil {
			return nil, err
		}
	}
	return cfs, nil
}

func (c *PreprocessingEmbed) scanFile(h func(header *tar.Header, reader io.Reader) (error, bool)) error {
	fp, err := c.FS.Open(c.sourceFileName)
	if err != nil {
		return utils.Errorf("open file %s failed: %v", c.sourceFileName, err)
	}
	defer fp.Close()
	gzReader, err := gzip.NewReader(fp)
	if err != nil {
		return err
	}
	defer gzReader.Close()
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			err, ok := h(header, tarReader)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
	}
	return nil
}

// ReadFile override embed.FS.ReadFile, if file is compress file, return decompress data
func (c *PreprocessingEmbed) ReadFile(name string) ([]byte, error) {
	var successful bool
	var content []byte
	if c.EnableCache {
		if c.cacheFile == nil {
			return nil, utils.Errorf("cacheFile is nil")
		}
		if data, ok := c.cacheFile[name]; ok {
			successful = true
			content = data
		}
	} else {
		err := c.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
			buf := &bytes.Buffer{}
			if _, err := io.Copy(buf, reader); err != nil {
				return err, true
			}
			successful = true
			content = buf.Bytes()
			return nil, false
		})
		if err != nil {
			return nil, err
		}
	}
	if successful {
		return content, nil
	}
	return nil, errors.New("file does not exist")
}
