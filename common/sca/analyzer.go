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

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/lazyfile"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const (
	opq string = ".wh..wh..opq"
	wh  string = ".wh."
)

type ScanConfig struct {
	endpoint        string
	numWorkers      int
	scanMode        analyzer.ScanMode
	usedAnalyzers   []analyzer.TypAnalyzer
	customAnalyzers []analyzer.Analyzer
	fs              fi.FileSystem
}

type ScanOption func(*ScanConfig)

func NewConfig() *ScanConfig {
	return &ScanConfig{
		numWorkers:    5,
		endpoint:      "",
		scanMode:      analyzer.AllMode,
		usedAnalyzers: []analyzer.TypAnalyzer{},
	}
}

// customAnalyzer 注册一个自定义 SCA 分析器，通过 matchFunc 决定是否处理某文件、analyzeFunc 产出软件包结果
// 在 yak 中通过 sca.customAnalyzer 调用
// 参数:
//   - matchFunc: 文件匹配函数，返回非 0 表示该文件由本分析器处理
//   - analyzeFunc: 分析函数，返回识别到的自定义软件包列表
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：注册一个自定义分析器
// opt = sca.customAnalyzer(func(info) { return 0 }, func(fi, others) { return [] })
// ```
func _withCustomAnalyzer(matchFunc func(info analyzer.MatchInfo) int, analyzeFunc func(fi *analyzer.FileInfo, otherFi map[string]*analyzer.FileInfo) []*analyzer.CustomPackage) ScanOption {
	return func(c *ScanConfig) {
		c.customAnalyzers = append(c.customAnalyzers, analyzer.NewCustomAnalyzer(matchFunc, analyzeFunc))
	}
}

// endpoint 设置扫描 Docker 镜像/容器时使用的 Docker Endpoint 地址
// 在 yak 中通过 sca.endpoint 调用
// 参数:
//   - endpoint: Docker Endpoint 地址，如 unix:///var/run/docker.sock
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定 Docker Endpoint
// pkgs = sca.ScanImageFromContext("nginx:latest", sca.endpoint("unix:///var/run/docker.sock"))~
// ```
func _withEndPoint(endpoint string) ScanOption {
	return func(c *ScanConfig) {
		c.endpoint = endpoint
	}
}

// scanMode 设置扫描模式，控制识别全部成分、仅系统包或仅语言依赖
// 在 yak 中通过 sca.scanMode 调用，取值如 sca.MODE_ALL、sca.MODE_PKG、sca.MODE_LANGUAGE
// 参数:
//   - mode: 扫描模式
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：仅扫描语言依赖
// pkgs = sca.ScanLocalFilesystem("/path/to/project", sca.scanMode(sca.MODE_LANGUAGE))~
// ```
func _withScanMode(mode analyzer.ScanMode) ScanOption {
	return func(c *ScanConfig) {
		c.scanMode |= mode
	}
}

// concurrent 设置扫描时的并发 worker 数量
// 在 yak 中通过 sca.concurrent 调用
// 参数:
//   - n: 并发 worker 数量
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：以 10 并发扫描本地目录
// pkgs = sca.ScanLocalFilesystem("/path/to/project", sca.concurrent(10))~
// ```
func _withConcurrent(n int) ScanOption {
	return func(c *ScanConfig) {
		c.numWorkers = n
	}
}

// analyzers 指定本次扫描启用的分析器类型，仅运行所列分析器
// 在 yak 中通过 sca.analyzers 调用，取值如 sca.ANALYZER_TYPE_JAVA_POM、sca.ANALYZER_TYPE_NODE_NPM
// 参数:
//   - a: 一个或多个分析器类型
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：仅启用 Java POM 分析器
// pkgs = sca.ScanLocalFilesystem("/path/to/project", sca.analyzers(sca.ANALYZER_TYPE_JAVA_POM))~
// ```
func _withAnalayzers(a ...analyzer.TypAnalyzer) ScanOption {
	return func(c *ScanConfig) {
		c.usedAnalyzers = append(c.usedAnalyzers, a...)
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

func walkFS(config *ScanConfig, pathStr string, handler walkFunc) error {
	if config.fs == nil {
		config.fs = filesys.NewLocalFs()
	}

	_, isLocal := config.fs.(*filesys.LocalFs)
	var startPath string
	if isLocal {
		startPath = pathStr
	} else {
		startPath = "."
	}

	return fs.WalkDir(config.fs, startPath, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return utils.Errorf("failed to walk the directory: %v", err)
		}
		if d.IsDir() {
			log.Debugf("skipping the directory: %s", filePath)
			return nil
		}

		statsInfo, err := d.Info()
		if err != nil {
			return err
		}

		f := lazyfile.LazyOpenStreamByFilePath(config.fs, filePath)
		defer f.Close()
		return handler(filePath, statsInfo, f)
	})
}

func walkGitCommit(repoDir string, handler walkFunc) error {
	r, err := git.PlainOpen(repoDir)
	if err != nil {
		return utils.Wrap(err, "failed to open the git repository")
	}

	iter, err := r.CommitObjects()
	if err != nil {
		return utils.Wrap(err, "failed to get the commit objects")
	}

	iter.ForEach(func(c *object.Commit) error {
		files, err := c.Files()
		if err != nil {
			return utils.Wrapf(err, "failed to get commit[%s] file", c.Hash)
		}

		files.ForEach(func(f *object.File) error {
			r, err := f.Reader()
			if err != nil {
				return utils.Wrap(err, "failed to get the reader")
			}
			defer r.Close()
			fileMode, err := f.Mode.ToOSFileMode()
			if err != nil {
				return utils.Wrap(err, "failed to convert file mode")
			}

			return handler(f.Name, lazyfile.NewFileInfo(f.Name, f.Size, fileMode), r)
		})

		return nil
	})

	return nil
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

func scanGitRepo(repoDir string, config *ScanConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode, config.usedAnalyzers, config.customAnalyzers)

	// match file
	err := walkGitCommit(repoDir, func(path string, fi fs.FileInfo, r io.Reader) error {
		if err := ag.Match(path, fi, r); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	wg := new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

func scanDockerImage(imageFile *os.File, config *ScanConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode, config.usedAnalyzers, config.customAnalyzers)

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

	wg := new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

func scanContainer(rc io.ReadCloser, config *ScanConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode, config.usedAnalyzers, config.customAnalyzers)

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

	wg := new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

func scanFS(fsPath string, config *ScanConfig) ([]*dxtypes.Package, error) {
	ag := analyzer.NewAnalyzerGroup(config.numWorkers, config.scanMode, config.usedAnalyzers, config.customAnalyzers, config.fs)
	// match file
	err := walkFS(config, fsPath, func(path string, fi fs.FileInfo, r io.Reader) error {
		if err := ag.Match(path, fi, r); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	wg := new(sync.WaitGroup)

	// analyzer-consumer
	ag.Consume(wg)

	// analyzer-productor
	ag.Analyze()

	wg.Wait()
	ag.Clear()
	return ag.Packages(), nil
}

// ScanLocalFilesystem 扫描本地文件系统目录，识别其中的软件成分(SCA)，返回检测到的软件包列表
// 在 yak 中通过 sca.ScanLocalFilesystem 调用，会根据各类包管理器清单(如 package.json、go.mod 等)解析依赖
// 参数:
//   - p: 待扫描的本地目录路径
//   - opts: 可选配置项，如 sca.concurrent、sca.scanMode、sca.analyzers
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：扫描本地项目目录的软件成分
// pkgs = sca.ScanLocalFilesystem("/path/to/project")~
//
//	for pkg = range pkgs {
//	    println(pkg.Name, pkg.Version)
//	}
//
// ```
func ScanLocalFilesystem(p string, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
	for _, opt := range opts {
		opt(config)
	}
	config.fs = filesys.NewLocalFs()
	return scanFS(p, config)
}

// ScanFilesystem 扫描给定的文件系统接口对象，识别其中的软件成分(SCA)，返回检测到的软件包列表
// 在 yak 中通过 sca.ScanFilesystem 调用，可配合 filesys 包构造的各类文件系统使用
// 参数:
//   - p: 实现 FileSystem 接口的文件系统对象(如 filesys.NewLocalFs() 等)
//   - opts: 可选配置项
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，输入文件系统为空或扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：扫描任意文件系统接口对象
// fs = filesys.NewLocalFs()
// pkgs = sca.ScanFilesystem(fs)~
// println("packages:", len(pkgs))
// ```
func ScanFilesystem(p fi.FileSystem, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
	config.fs = p
	for _, opt := range opts {
		opt(config)
	}
	if config.fs == nil {
		return nil, utils.Errorf("ScanFilesystem need fs.FS interface as input, try filesys.New... instead: %T", p)
	}
	return scanFS(".", config)
}

// ScanGitRepo 遍历本地 Git 仓库的全部提交历史，识别各历史版本中的软件成分(SCA)
// 在 yak 中通过 sca.ScanGitRepo 调用
// 参数:
//   - repoDir: 本地 Git 仓库目录路径
//   - opts: 可选配置项
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：扫描本地 Git 仓库历史中的软件成分
// pkgs = sca.ScanGitRepo("/path/to/repo")~
// println("packages:", len(pkgs))
// ```
func ScanGitRepo(repoDir string, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
	for _, opt := range opts {
		opt(config)
	}
	return scanGitRepo(repoDir, config)
}

// ScanDockerContainerFromContext 通过 Docker API 导出指定容器(包括其挂载卷)，识别其中的软件成分(SCA)
// 在 yak 中通过 sca.ScanContainerFromContext 调用，依赖本地或远程 Docker 环境
// 参数:
//   - containerID: 目标容器 ID 或名称
//   - opts: 可选配置项，如 sca.endpoint 指定 Docker Endpoint
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，连接 Docker 或扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖 Docker 环境，扫描运行中的容器
// pkgs = sca.ScanContainerFromContext("my-container", sca.endpoint("unix:///var/run/docker.sock"))~
// println("packages:", len(pkgs))
// ```
func ScanDockerContainerFromContext(containerID string, opts ...ScanOption) (pkgs []*dxtypes.Package, err error) {
	config := NewConfig()
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
		fsPkgs, err := scanFS(source, config)
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
	containerPkgs, err := scanContainer(rc, config)
	if err != nil {
		return pkgs, err
	}
	pkgs = append(pkgs, containerPkgs...)

	return analyzer.MergePackages(pkgs), nil
}

// ScanDockerImageFromContext 通过 Docker API 拉取/导出指定镜像，识别其中的软件成分(SCA)
// 在 yak 中通过 sca.ScanImageFromContext 调用，依赖本地或远程 Docker 环境
// 参数:
//   - imageID: 目标镜像 ID 或名称
//   - opts: 可选配置项，如 sca.endpoint 指定 Docker Endpoint
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，连接 Docker 或扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖 Docker 环境，扫描镜像
// pkgs = sca.ScanImageFromContext("nginx:latest")~
// println("packages:", len(pkgs))
// ```
func ScanDockerImageFromContext(imageID string, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
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

	client, err := NewDockerClient(config.endpoint)
	defer client.Close()
	if err != nil {
		return nil, err
	}
	if err = saveImage(client, imageID, f); err != nil {
		return nil, err
	}

	pkgs, err := scanDockerImage(f, config)
	if err != nil {
		return nil, utils.Errorf("failed to scan image[%s] : %v", imageID, err)
	}

	return analyzer.MergePackages(pkgs), nil
}

// ScanDockerImageFromFile 扫描本地保存的 Docker 镜像 tar 文件，识别其中的软件成分(SCA)
// 在 yak 中通过 sca.ScanImageFromFile 调用，无需连接 Docker 守护进程
// 参数:
//   - path: 本地镜像 tar 文件路径(通常由 docker save 导出)
//   - opts: 可选配置项
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，文件打开或扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：扫描 docker save 导出的镜像文件
// pkgs = sca.ScanImageFromFile("/path/to/image.tar")~
// println("packages:", len(pkgs))
// ```
func ScanDockerImageFromFile(path string, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
	for _, opt := range opts {
		opt(config)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, utils.Errorf("unable to open file: %v", err)
	}
	defer f.Close()

	pkgs, err := scanDockerImage(f, config)
	if err != nil {
		return nil, utils.Errorf("failed to scan image from filepath[%s] : %v", path, err)
	}

	return analyzer.MergePackages(pkgs), nil
}
