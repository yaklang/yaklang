package sca

import (
	"archive/tar"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	opq string = ".wh..wh..opq"
	wh  string = ".wh."
)

type dockerContextConfig struct {
	endpoint   string
	numWorkers int
	scanMode   analyzer.ScanMode
}

type dockerContextOption func(*dockerContextConfig)

func NewDockerContextConfig() *dockerContextConfig {
	return &dockerContextConfig{
		numWorkers: 5,
		endpoint:   "",
		scanMode:   analyzer.AllMode,
	}
}

func _withEndPoint(endpoint string) dockerContextOption {
	return func(c *dockerContextConfig) {
		c.endpoint = endpoint
	}
}

func _withScanMode(mode analyzer.ScanMode) dockerContextOption {
	return func(c *dockerContextConfig) {
		c.scanMode |= mode
	}
}

func saveImageFromContext(host, imageID string, f io.Writer) error {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if host != "" {
		// adding host parameter to the last assuming it will pick up more preference
		opts = append(opts, client.WithHost(host))
	}
	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return utils.Errorf("failed to initialize a docker client: %v", err)
	}
	defer c.Close()

	// Store the tarball in local filesystem and return a new reader into the bytes each time we need to access something.
	ctx := context.Background()
	rc, err := c.ImageSave(ctx, []string{imageID})
	if err != nil {
		return utils.Errorf("unable to export the image: %v", err)
	}
	defer rc.Close()

	if _, err = io.Copy(f, rc); err != nil {
		return utils.Errorf("failed to copy the image: %v", err)
	}
	return nil
}

type walkFunc func(string, fs.FileInfo, io.Reader) error

func walkImage(image *os.File, walkFunc walkFunc) error {
	img, err := tarball.ImageFromPath(image.Name(), nil)
	if err != nil {
		return utils.Errorf("failed to initialize the struct from the temporary file: %v", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return utils.Errorf("failed to get layers: %v", err)
	}

	layers = lo.Reverse(layers)

	for _, layer := range layers {
		rc, err := layer.Uncompressed()
		if err != nil {
			// continue
			return utils.Errorf("unable to get  uncompressed layer: %v", err)
		}
		tr := tar.NewReader(rc)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return utils.Errorf("failed to extract the archive: %v", err)
			}

			// filepath.Clean cannot be used since tar file paths should be OS-agnostic.
			filePath := path.Clean(hdr.Name)
			// filePath = strings.TrimLeft(filePath, "/")
			_, fileName := path.Split(filePath)

			// "OverlayFS" creates a set of hidden files beginning with ".wh." (which stands for "whiteout") to record changes made to the underlying filesystem.
			// e.g. etc/.wh..wh..opq
			if opq == fileName {
				// opqDirs = append(opqDirs, fileDir)
				continue
			}
			// etc/.wh.hostname
			if strings.HasPrefix(fileName, wh) {
				// name := strings.TrimPrefix(fileName, wh)
				// fpath := path.Join(fileDir, name)
				// whFiles = append(whFiles, fpath)
				continue
			}
			if err = walkFunc(filePath, hdr.FileInfo(), tr); err != nil {
				return err
			}

		}
	}
	return nil
}

func loadDockerImage(imageFile *os.File, config dockerContextConfig) ([]types.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode)
	ag.Append(
		analyzer.NewDpkgAnalyzer(),
		analyzer.NewApkAnalyzer(),
		analyzer.NewRPMAnalyzer(),
	)

	// ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	// up consumer
	ag.Consume(&wg)
	// close

	// producter
	err := walkImage(imageFile, func(path string, fi fs.FileInfo, r io.Reader) error {
		if err := ag.Analyze(path, fi, r); err != nil {
			return err
		}
		return nil
	})
	ag.Close()

	if err != nil {
		return nil, err
	}

	wg.Wait()
	if err = ag.Error(); err != nil {
		return nil, err
	}
	return ag.Packages(), nil
}

func LoadDockerImageFromContext(imageID string, opts ...dockerContextOption) ([]types.Package, error) {
	config := NewDockerContextConfig()
	for _, opt := range opts {
		opt(config)
	}

	f, err := os.CreateTemp("", "fanal-*")
	if err != nil {
		return nil, utils.Errorf("failed to create a temporary file")
	}
	defer func() {
		name := f.Name()
		f.Close()
		os.Remove(name)
	}()

	if err = saveImageFromContext(config.endpoint, imageID, f); err != nil {
		return nil, err

	}

	// defer f.Close()

	return loadDockerImage(f, *config)
}

func LoadDockerImageFromFile(path string, opts ...dockerContextOption) ([]types.Package, error) {
	config := NewDockerContextConfig()
	for _, opt := range opts {
		opt(config)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, utils.Errorf("unable to open file: %v", err)
	}
	defer f.Close()

	return loadDockerImage(f, *config)
}
