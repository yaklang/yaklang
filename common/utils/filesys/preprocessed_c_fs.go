package filesys

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PreprocessedCFS struct {
	underlying  fi.FileSystem
	tempDir     string
	includeDirs []string
	enabled     bool
}

var _ fi.FileSystem = (*PreprocessedCFS)(nil)

// NewPreprocessedCFs creates a new C preprocessor filesystem wrapper
func NewPreprocessedCFs(underlying fi.FileSystem) (*PreprocessedCFS, error) {
	fs := &PreprocessedCFS{
		underlying: underlying,
		enabled:    true,
	}

	tmpDir, err := os.MkdirTemp("", "c_headers_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	fs.tempDir = tmpDir

	// Copy all .h files
	if err := fs.setupHeaderFiles(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to setup header files: %w", err)
	}

	return fs, nil
}

var commonCLibraries = []string{
	// C 标准库与语言扩展
	"assert.h", "complex.h", "ctype.h", "errno.h", "fenv.h", "float.h", "inttypes.h",
	"iso646.h", "limits.h", "locale.h", "math.h", "setjmp.h", "signal.h", "stdalign.h",
	"stdarg.h", "stdatomic.h", "stdbool.h", "stddef.h", "stdint.h", "stdio.h", "stdlib.h",
	"stdnoreturn.h", "string.h", "tgmath.h", "threads.h", "time.h", "uchar.h", "wchar.h",
	"wctype.h", "malloc.h", "libclidef.h", "stsdef.h",

	// 字符编码与国际化
	"iconv.h", "libintl.h",

	// 通用工具与数据结构
	"apr_optional.h", "apr_optional_hooks.h", "apr_strings.h", "glib.h", "khash.h", "kvec.h", "uthash.h", "utarray.h",

	// POSIX / Unix 系统接口
	"aio.h", "arpa/inet.h", "descrip.h", "dirent.h", "dlfcn.h", "fcntl.h", "ifaddrs.h",
	"mqueue.h", "net/if.h", "netdb.h", "netinet/in.h", "netinet/ip.h", "netinet/ip6.h",
	"netinet/tcp.h", "poll.h", "pthread.h", "pty.h", "semaphore.h", "sched.h", "spawn.h",
	"sys/epoll.h", "sys/event.h", "sys/eventfd.h", "sys/inotify.h", "sys/ioctl.h",
	"sys/ipc.h", "sys/mman.h", "sys/msg.h", "sys/poll.h", "sys/resource.h", "sys/select.h",
	"sys/sem.h", "sys/shm.h", "sys/socket.h", "sys/stat.h", "sys/statvfs.h", "sys/syscall.h",
	"sys/time.h", "sys/times.h", "sys/types.h", "sys/uio.h", "sys/un.h", "sys/utsname.h",
	"sys/wait.h", "sys/timerfd.h", "syslog.h", "termios.h", "ucontext.h", "unistd.h",

	// 其他平台与系统扩展
	"lnmdef.h", "qadrt.h",

	// Windows 平台接口
	"_mingw.h", "bcrypt.h", "direct.h", "io.h", "process.h", "synchapi.h", "tchar.h", "windows.h",
	"winerror.h", "wincrypt.h", "winsock2.h", "ws2tcpip.h", "w32api.h",

	// 并行与并发
	"dispatch/dispatch.h", "mpi.h", "omp.h",

	// 网络与异步 I/O
	"ares.h", "curl/curl.h", "ev.h", "event2/event.h", "event2/event_struct.h", "http_parser.h",
	"libcoap-2/coap.h", "libssh/libssh.h", "libssh2.h", "libuv/uv.h", "microhttpd.h", "mosquitto.h",
	"nanomsg/nn.h", "nghttp2/nghttp2.h", "pcap/pcap.h", "rdma/rdma_cma.h", "uv.h",
	"websocketpp/client.hpp", "zmq.h",

	// 安全与密码学
	"gnutls/gnutls.h", "gnutls/x509.h", "libgcrypt.h", "libsodium.h", "mbedtls/ssl.h",
	"openssl/aes.h", "openssl/conf.h", "openssl/crypto.h", "openssl/dh.h", "openssl/err.h",
	"openssl/evp.h", "openssl/opensslv.h", "openssl/pem.h", "openssl/rand.h", "openssl/rsa.h", "openssl/sha.h",
	"openssl/ssl.h", "openssl/x509.h", "openssl/configuration.h", "openssl/safestack.h", "openssl/ui.h", "openssl/bio.h", "openssl/asn1.h", "pkcs11.h", "sodium.h", "wolfssl/options.h",

	// 压缩与归档
	"archive.h", "archive_entry.h", "brotli/decode.h", "brotli/encode.h", "bzlib.h",
	"libtar.h", "lz4.h", "lzma.h", "minizip/unzip.h", "zconf.h", "zlib.h", "zstd.h",

	// 数据库与存储
	"hiredis/hiredis.h", "leveldb/c.h", "lmdb.h", "mongoc/mongoc.h", "mysql/mysql.h",
	"postgresql/libpq-fe.h", "rdkafka.h", "rocksdb/c.h", "sqlite3.h", "wiredtiger.h",

	// 序列化与数据交换
	"avro.h", "bson/bson.h", "cJSON.h", "cbor.h", "expat.h", "flatbuffers/flatbuffers.h",
	"htmlstreamparser.h", "jansson.h", "json-c/json.h", "libxml/HTMLparser.h",
	"libxml/parser.h", "libxml/uri.h", "libxml/xpath.h", "msgpack.h", "protobuf-c/protobuf-c.h",
	"rapidjson/document.h", "tidy/tidy.h", "tidy/tidybuffio.h", "yaml.h",

	// 科学计算与数值分析
	"cblas.h", "fftw3.h", "gsl/gsl_math.h", "gsl/gsl_matrix.h", "gsl/gsl_vector.h",
	"lapacke.h", "mkl.h",

	// 图形界面与桌面应用
	"cairo/cairo.h", "gdk/gdk.h", "gtk/gtk.h",

	// 图形渲染与多媒体
	"GL/glew.h", "GL/gl.h", "GL/glu.h", "GLFW/glfw3.h", "GLES2/gl2.h",
	"OpenCL/opencl.h", "SDL2/SDL.h", "SDL2/SDL_image.h", "SDL2/SDL_mixer.h",
	"SDL2/SDL_ttf.h", "allegro5/allegro.h", "vulkan/vulkan.h",

	// 图像与视频处理
	"MagickWand/MagickWand.h", "gif_lib.h", "jpeg/jpeglib.h", "libavcodec/avcodec.h",
	"libavdevice/avdevice.h", "libavfilter/avfilter.h", "libavformat/avformat.h",
	"libavutil/avutil.h", "libpostproc/postprocess.h", "libswresample/swresample.h",
	"libswscale/swscale.h", "opencv2/core/core_c.h", "opencv2/highgui/highgui_c.h",
	"opencv2/imgproc/imgproc_c.h", "openjpeg.h", "png.h", "tiffio.h", "turbojpeg.h",
	"webp/decode.h", "webp/encode.h",

	// 音频处理
	"alsa/asoundlib.h", "ao/ao.h", "fdk-aac/aacdecoder_lib.h", "jack/jack.h", "mpg123.h",
	"openal/al.h", "openal/alc.h", "opus/opus.h", "portaudio.h", "pulse/pulseaudio.h",
	"sndfile.h", "speex/speex.h", "vorbis/vorbisfile.h",

	// 命令行与终端
	"ncurses.h", "panel.h", "readline/history.h", "readline/readline.h", "regex.h", "term.h",

	// 调试与性能分析
	"execinfo.h", "gperftools/profiler.h", "libunwind.h", "sanitizer/asan_interface.h",
	"valgrind/valgrind.h",

	// 嵌入式与实时系统
	"FreeRTOS.h", "cmsis_os.h", "lwip/init.h", "zephyr/kernel.h",

	// 机器学习与 AI 接口
	"mxnet/c_api.h", "onnxruntime_c_api.h", "tensorflow/c/c_api.h", "tflite/c/c_api.h",
}

func (f *PreprocessedCFS) setupHeaderFiles() error {
	headerDirs := make(map[string]bool)

	var walkDir func(string) error
	walkDir = func(dir string) error {
		entries, err := f.underlying.ReadDir(dir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			filePath := f.underlying.Join(dir, entry.Name())

			if entry.IsDir() {
				walkDir(filePath)
			} else if f.underlying.Ext(entry.Name()) == ".h" {
				if content, err := f.underlying.ReadFile(filePath); err == nil {
					filtered := filterSystemIncludes(string(content))
					relPath := strings.TrimPrefix(filePath, ".")
					relPath = strings.TrimPrefix(relPath, string(f.underlying.GetSeparators()))
					targetPath := filepath.Join(f.tempDir, relPath)
					targetDir := filepath.Dir(targetPath)

					os.MkdirAll(targetDir, 0755)
					os.WriteFile(targetPath, []byte(filtered), 0644)
					headerDirs[targetDir] = true
				}
			}
		}
		return nil
	}

	if err := walkDir("."); err != nil {
		return err
	}

	f.includeDirs = make([]string, 0, len(headerDirs)+1)
	f.includeDirs = append(f.includeDirs, f.tempDir)
	for dir := range headerDirs {
		if dir != f.tempDir {
			f.includeDirs = append(f.includeDirs, dir)
		}
	}

	var copyIncludeDir func(string, string) error
	copyIncludeDir = func(srcDir, dstDir string) error {
		for _, std := range commonCLibraries {
			targetPath := filepath.Join(dstDir, std)
			dirPath := filepath.Dir(targetPath)

			if _, err := os.Stat(dirPath); err != nil {
				if os.IsNotExist(err) {
					if err := os.MkdirAll(dirPath, 0755); err != nil {
						return err
					}
				} else {
					return err
				}
			}

			if _, err := os.Stat(targetPath); err != nil {
				if os.IsNotExist(err) {
					if err := os.WriteFile(targetPath, []byte{}, 0644); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}

		entries, err := f.underlying.ReadDir(srcDir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			srcPath := f.underlying.Join(srcDir, entry.Name())
			dstPath := filepath.Join(dstDir, entry.Name())

			if entry.IsDir() {
				os.MkdirAll(dstPath, 0755)
				headerDirs[dstPath] = true
				copyIncludeDir(srcPath, dstPath)
			} else {
				if content, err := f.underlying.ReadFile(srcPath); err == nil {
					if f.underlying.Ext(entry.Name()) == ".h" {
						content = []byte(filterSystemIncludes(string(content)))
					}
					os.WriteFile(dstPath, content, 0644)
					headerDirs[filepath.Dir(dstPath)] = true
				}
			}
		}
		return nil
	}

	includeDir := filepath.Join(f.tempDir, "include")
	os.MkdirAll(includeDir, 0755)
	headerDirs[includeDir] = true
	copyIncludeDir("include", includeDir)

	f.includeDirs = make([]string, 0, len(headerDirs)+1)
	f.includeDirs = append(f.includeDirs, f.tempDir)
	for dir := range headerDirs {
		if dir != f.tempDir {
			f.includeDirs = append(f.includeDirs, dir)
		}
	}

	return nil
}

// PreprocessCSource performs C macro preprocessing on source code
func (f *PreprocessedCFS) PreprocessCSource(src string) (string, error) {
	/* TODO: 未来改进：
	   1. 将 gcc/clang 集成到项目中（可选）
	   2. 提供在不同平台上构建 gcc/clang 的脚本
	   3. 添加编译选项，让用户决定是否自动扩展宏
	   4. 支持用户指定额外的 include 路径（-I 选项）
	*/
	var preprocessorCmd string

	candidates := []string{"gcc", "clang", "cc"}
	for _, cmd := range candidates {
		if _, err := exec.LookPath(cmd); err == nil {
			preprocessorCmd = cmd
			break
		}
	}

	if preprocessorCmd == "" {
		return "", fmt.Errorf("c preprocessor not found: please install gcc, clang, or compatible C compiler (platform: %s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	tmpFile, err := os.CreateTemp(f.tempDir, "c_preprocess_*.c")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFileName := tmpFile.Name()
	defer os.Remove(tmpFileName)

	if _, err := tmpFile.WriteString(src); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write source to temp file: %w", err)
	}
	tmpFile.Close()

	preprocessorArgs := []string{
		"-E",
		"-P",
		"-nostdinc",
		"-Wno-everything",
	}

	// Add all include directories
	for _, includeDir := range f.includeDirs {
		preprocessorArgs = append(preprocessorArgs, "-I", includeDir)
	}

	preprocessorArgs = append(preprocessorArgs, tmpFileName)

	cmd := exec.Command(preprocessorCmd, preprocessorArgs...)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		if len(outputStr) > 500 {
			outputStr = outputStr[:500] + "... (truncated)"
		}
		return src, fmt.Errorf("preprocessor failed: %w\nOutput: %s", err, outputStr)
	}

	return outputStr, nil
}

func filterSystemIncludes(src string) string {
	var builder strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(src))
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#include") {
			continue
		}
		if !firstLine {
			builder.WriteString("\n")
		}
		builder.WriteString(line)
		firstLine = false
	}
	if err := scanner.Err(); err != nil {
		return src
	}
	if firstLine {
		return src
	}
	return builder.String()
}

func (f *PreprocessedCFS) ReadFile(name string) ([]byte, error) {
	data, err := f.underlying.ReadFile(name)
	if err != nil {
		return nil, err
	}

	if f.enabled && (strings.HasSuffix(strings.ToLower(name), ".c") || strings.HasSuffix(strings.ToLower(name), ".h")) {
		preprocessed, err := f.PreprocessCSource(string(data))
		if err != nil {
			log.Warnf("C macro preprocessing failed for %s: %v, using original source", name, err)
			return data, nil
		}
		return []byte(preprocessed), nil
	}

	return data, nil
}

func (f *PreprocessedCFS) Cleanup() {
	if f.tempDir != "" {
		os.RemoveAll(f.tempDir)
		f.tempDir = ""
	}
}

func (f *PreprocessedCFS) SetEnabled(enabled bool) {
	f.enabled = enabled
}

func (f *PreprocessedCFS) GetTempDir() string {
	return f.tempDir
}

func (f *PreprocessedCFS) Open(name string) (fs.File, error) {
	return f.underlying.Open(name)
}

func (f *PreprocessedCFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.underlying.OpenFile(name, flag, perm)
}

func (f *PreprocessedCFS) Stat(name string) (fs.FileInfo, error) {
	return f.underlying.Stat(name)
}

func (f *PreprocessedCFS) ReadDir(dirname string) ([]fs.DirEntry, error) {
	return f.underlying.ReadDir(dirname)
}

func (f *PreprocessedCFS) GetSeparators() rune {
	return f.underlying.GetSeparators()
}

func (f *PreprocessedCFS) Join(paths ...string) string {
	return f.underlying.Join(paths...)
}

func (f *PreprocessedCFS) IsAbs(name string) bool {
	return f.underlying.IsAbs(name)
}

func (f *PreprocessedCFS) Getwd() (string, error) {
	return f.underlying.Getwd()
}

func (f *PreprocessedCFS) Exists(path string) (bool, error) {
	return f.underlying.Exists(path)
}

func (f *PreprocessedCFS) Rename(old string, new string) error {
	return f.underlying.Rename(old, new)
}

func (f *PreprocessedCFS) Rel(base string, target string) (string, error) {
	return f.underlying.Rel(base, target)
}

func (f *PreprocessedCFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return f.underlying.WriteFile(name, data, perm)
}

func (f *PreprocessedCFS) Delete(name string) error {
	return f.underlying.Delete(name)
}

func (f *PreprocessedCFS) MkdirAll(name string, perm os.FileMode) error {
	return f.underlying.MkdirAll(name, perm)
}

func (f *PreprocessedCFS) String() string {
	underlyingStr := "FileSystem"
	if stringer, ok := f.underlying.(fmt.Stringer); ok {
		underlyingStr = stringer.String()
	}
	return fmt.Sprintf("PreprocessedCFS{underlying: %s, tempDir: %s}", underlyingStr, f.tempDir)
}

func (f *PreprocessedCFS) Root() string {
	if rooter, ok := f.underlying.(interface{ Root() string }); ok {
		return rooter.Root()
	}
	return ""
}

func (f *PreprocessedCFS) ExtraInfo(path string) map[string]any {
	return f.underlying.ExtraInfo(path)
}

func (f *PreprocessedCFS) Base(p string) string {
	return f.underlying.Base(p)
}

func (f *PreprocessedCFS) PathSplit(s string) (string, string) {
	return f.underlying.PathSplit(s)
}

func (f *PreprocessedCFS) Ext(s string) string {
	return f.underlying.Ext(s)
}
