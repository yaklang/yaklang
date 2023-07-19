package sca

import (
	"archive/tar"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/lazyfile"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/analyzer"
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

func _withConcurrent(n int) dockerContextOption {
	return func(c *dockerContextConfig) {
		c.numWorkers = n
	}
}

func NewDockerClient(host string) (*client.Client, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}
	if host != "" {
		// adding host parameter to the last assuming it will pick up more preference
		opts = append(opts, client.WithHost(host))
	} else {
		opts = append(opts, client.FromEnv)
	}
	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, utils.Errorf("failed to initialize a docker client: %v", err)
	}
	return c, nil
}

func saveImage(c *client.Client, imageID string, f io.Writer) error {
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

func exportContainer(c *client.Client, containerID string) (io.ReadCloser, error) {
	ctx := context.Background()
	rc, err := c.ContainerExport(ctx, containerID)
	if err != nil {
		return nil, utils.Errorf("unable to export the container: %v", err)
	}

	return rc, nil
}

func getContainerMountFSSource(c *client.Client, containerID string) ([]string, error) {
	ctx := context.Background()
	containerJSON, err := c.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, utils.Errorf("unable to get container mount FS: %v", err)
	}

	return lo.Map(containerJSON.Mounts, func(item types.MountPoint, _ int) string {
		return item.Source
	}), nil
}

type walkFunc func(string, fs.FileInfo, io.Reader) error

func walkFS(pathStr string, handler walkFunc) error {
	var startPath = pathStr
	var err error
	if !filepath.IsAbs(pathStr) {
		startPath, err = filepath.Abs(pathStr)
		if err != nil {
			return utils.Errorf("cannot fetch the absolute path: %v", err)
		}
	}

	return fs.WalkDir(os.DirFS(startPath), ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return utils.Errorf("failed to walk the directory: %v", err)
		}
		if d.IsDir() {
			log.Debugf("skipping the directory: %s", filePath)
			return nil
		}
		filePath = path.Join(startPath, filePath)

		statsInfo, err := d.Info()
		if err != nil {
			return err
		}

		f := lazyfile.LazyOpenStreamByFilePath(filePath)
		defer f.Close()
		return handler(filePath, statsInfo, f)
	})
}

func walkImage(image *os.File, handler walkFunc) error {
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
		walkLayer(rc, handler)
	}
	return nil
}

func walkLayer(rc io.ReadCloser, handler walkFunc) error {
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
		if err = handler(filePath, hdr.FileInfo(), tr); err != nil {
			return err
		}

	}
	return nil
}

func scanDockerImage(imageFile *os.File, config dockerContextConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode)

	// match file
	err := walkImage(imageFile, func(path string, fi fs.FileInfo, r io.Reader) error {
		if err := ag.Match(path, fi, r); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var wg = new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

func scanContainer(rc io.ReadCloser, config dockerContextConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode)

	// match file
	err := walkLayer(rc, func(path string, fi fs.FileInfo, r io.Reader) error {
		if err := ag.Match(path, fi, r); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var wg = new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

func scanFS(fsPath string, config dockerContextConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode)

	// match file
	err := walkFS(fsPath, func(path string, fi fs.FileInfo, r io.Reader) error {
		if err := ag.Match(path, fi, r); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var wg = new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

func ScanDockerContainerFromContext(containerID string, opts ...dockerContextOption) (pkgs []*dxtypes.Package, err error) {
	// merge pkgs
	// defer func() {
	// 	if len(pkgs) > 0 {
	// 		pkgs =
	// 	}
	// }

	config := NewDockerContextConfig()
	for _, opt := range opts {
		opt(config)
	}

	// docker client
	c, err := NewDockerClient(config.endpoint)
	if err != nil {
		return nil, err
	}

	// get and scan mount FS
	sources, err := getContainerMountFSSource(c, containerID)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		fsPkgs, err := scanFS(source, *config)
		if err != nil {
			continue // ?
		}
		pkgs = append(pkgs, fsPkgs...)
	}

	// get and scan container fs
	rc, err := exportContainer(c, containerID)
	if err != nil {
		return nil, err
	}
	containerPkgs, err := scanContainer(rc, *config)

	if err != nil {
		return pkgs, err
	}
	pkgs = append(pkgs, containerPkgs...)

	return pkgs, nil
}

func ScanDockerImageFromContext(imageID string, opts ...dockerContextOption) ([]*dxtypes.Package, error) {
	config := NewDockerContextConfig()
	for _, opt := range opts {
		opt(config)
	}

	log.Infof("create temporary file to store the image: %s", imageID)
	f, err := os.CreateTemp("", "fanal-*")
	if err != nil {
		return nil, utils.Errorf("failed to create a temporary file")
	}
	defer func() {
		name := f.Name()
		f.Close()
		os.Remove(name)
	}()

	log.Infof("start to save the image: %s", imageID)
	client, err := NewDockerClient(config.endpoint)
	defer client.Close()
	if err != nil {
		return nil, err
	}
	if err = saveImage(client, imageID, f); err != nil {
		return nil, err
	}

	pkgs, err := scanDockerImage(f, *config)
	if err != nil {
		return nil, utils.Errorf("failed to scan image[%s] : %v", imageID, err)
	}

	return pkgs, nil
}

func ScanDockerImageFromFile(path string, opts ...dockerContextOption) ([]*dxtypes.Package, error) {
	config := NewDockerContextConfig()
	for _, opt := range opts {
		opt(config)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, utils.Errorf("unable to open file: %v", err)
	}
	defer f.Close()

	pkgs, err := scanDockerImage(f, *config)
	if err != nil {
		return nil, utils.Errorf("failed to scan image from filepath[%s] : %v", path, err)
	}

	return pkgs, nil
}
