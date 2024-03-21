package resources

import (
	"compress/gzip"
	"embed"
	"encoding/binary"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"path"
)

//go:embed *
var resourceFS embed.FS

// CompressFS is a simple tools to read file from embed.FS and gzip compress file
// only support ReadFile method, not support Open method
type CompressFS struct {
	*embed.FS
	cacheFile       map[string][]byte // key is virtual file name, value is file data
	compressFileDir map[string]string // key is virtual dir, value is compress file name
}

// NewCompressFS create a CompressFS instance
// fs is embed.FS instance, compressDirs is a map, key is virtual dir, value is compress file name
func NewCompressFS(fs *embed.FS, compressDirs map[string]string) *CompressFS {
	cfs := &CompressFS{
		FS:              fs,
		cacheFile:       map[string][]byte{},
		compressFileDir: map[string]string{},
	}

	for dir, compressFileName := range compressDirs {
		fp, err := fs.Open(compressFileName)
		if err != nil {
			log.Errorf("open file %s failed: %v", compressFileName, err)
			continue
		}
		reader, err := gzip.NewReader(fp)
		if err != nil {
			log.Errorf("gzip.NewReader failed: %v", err)
			continue
		}

		readDataBlock := func(l int) ([]byte, error) {
			data := make([]byte, l)
			_, err := io.ReadFull(reader, data)
			if err != nil {
				return nil, err
			}
			return data, nil
		}
		for {
			lenBytes, err := readDataBlock(4)
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Errorf("readDataBlock failed: %v", err)
				continue
			}
			l := binary.BigEndian.Uint32(lenBytes)
			data, err := readDataBlock(int(l))
			if err != nil {
				log.Errorf("readDataBlock failed: %v", err)
				continue
			}
			fileName := string(data)

			lenBytes, err = readDataBlock(4)
			if err != nil {
				log.Errorf("readDataBlock failed: %v", err)
				continue
			}
			l = binary.BigEndian.Uint32(lenBytes)
			data, err = readDataBlock(int(l))
			if err != nil {
				log.Errorf("readDataBlock failed: %v", err)
				continue
			}
			cfs.cacheFile[path.Join(dir, fileName)] = data
			cfs.compressFileDir[dir] = compressFileName
		}
	}
	return cfs
}

// ReadFile override embed.FS.ReadFile, if file is compress file, return decompress data
func (c *CompressFS) ReadFile(name string) ([]byte, error) {
	if data, ok := c.cacheFile[name]; ok {
		return data, nil
	}
	return c.FS.ReadFile(name)
}

var YsoResourceFS *CompressFS

func init() {
	YsoResourceFS = NewCompressFS(&resourceFS, map[string]string{"gadgets": path.Join("gadgets", "gadgets.bin")})
}
