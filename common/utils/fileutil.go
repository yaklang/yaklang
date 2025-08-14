package utils

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func GetFirstExistedFile(paths ...string) string {
	res, _ := GetFirstExistedFileE(paths...)
	return res
}

func GetFirstExistedFileE(paths ...string) (string, error) {
	var existedFile string
	for _, t := range paths {
		r, err := PathExists(t)
		if err != nil {
			continue
		}

		// 如果是目录，跳过，进行下一个判断
		if IsDir(t) {
			continue
		}

		if !r {
			continue
		}

		existedFile = t
		break
	}

	if existedFile != "" {
		return existedFile, nil
	}
	return "", Errorf("any path is not existed")
}

func GetFirstExistedPathE(paths ...string) (string, error) {
	var existedFile string
	for _, t := range paths {
		r, err := PathExists(t)
		if err != nil {
			continue
		}

		if !r {
			continue
		}

		existedFile = t
		break
	}

	if existedFile != "" {
		return existedFile, nil
	}
	return "", Errorf("any path is not existed")
}

func GetFirstExistedPath(paths ...string) string {
	r, _ := GetFirstExistedPathE(paths...)
	return r
}

func IsDir(path string) bool {
	if info, err := os.Stat(path); err != nil {
		return false
	} else {
		if info.IsDir() {
			return true
		}
		return false
	}
}

func IsFile(path string) bool {
	if info, err := os.Stat(path); err != nil {
		return false
	} else {
		if info.IsDir() {
			return false
		}
		return true
	}
}

func GetFirstExistedExecutablePath(paths ...string) string {
	r, _ := GetFirstExistedPathE(paths...)
	if r == "" {
		return ""
	}

	stats, err := os.Stat(r)
	if err != nil {
		return ""
	}

	if stats.Mode()&0o111 == 0 {
		return ""
	}

	return r
}

func GetExecutableFromEnv(cmd string) (string, error) {
	path, ok := os.LookupEnv("PATH")
	if !ok {
		return "", Errorf("PATH environment variable not found")
	}
	// windows 判断，补上.exe
	if runtime.GOOS == "windows" {
		cmd += ".exe"
	}

	for _, dir := range filepath.SplitList(path) {
		exePath := filepath.Join(dir, cmd)
		if _, err := os.Stat(exePath); err == nil {
			return exePath, nil
		}
	}

	return "", Errorf("command %s not found in PATH", cmd)
}

func CopyDirectory(source string, destination string, isMove bool) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 构建新路径
		refPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		newPath := filepath.Join(destination, refPath)

		if info.IsDir() {
			// 创建新的文件夹
			err := os.MkdirAll(newPath, info.Mode())
			if err != nil {
				return err
			}
		} else {
			// 复制文件
			err := CopyFile(path, newPath)
			if err != nil {
				return err
			}

			if isMove {
				// 删除源文件
				err = os.Remove(path)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func CopyDirectoryEx(source string,
	destination string,
	isMove bool,
	fs fi.FileSystem,
) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 构建新路径
		refPath, err := fs.Rel(source, path)
		if err != nil {
			return err
		}
		newPath := fs.Join(destination, refPath)

		if info.IsDir() {
			// 创建新的文件夹
			err := fs.MkdirAll(newPath, info.Mode())
			if err != nil {
				return err
			}
		} else {
			// 复制文件
			err := CopyFileEx(path, newPath, fs)
			if err != nil {
				return err
			}
			// 删除源文件
			if isMove {
				err = fs.Delete(path)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

type copyFileTask struct {
	path    string
	newPath string
}

func ConcurrentCopyDirectory(source string, destination string, threads int, isMove bool) error {
	wg := &sync.WaitGroup{}
	ch := make(chan copyFileTask)
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range ch {
				CopyFile(task.path, task.newPath)
				if isMove {
					os.Remove(task.path)
				}
			}
		}()
	}

	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 构建新路径
		refPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		newPath := filepath.Join(destination, refPath)

		if info.IsDir() {
			// 创建新的文件夹
			err := os.MkdirAll(newPath, info.Mode())
			if err != nil {
				return err
			}
		} else {
			ch <- copyFileTask{path: path, newPath: newPath}
		}

		return nil
	})
	close(ch)
	wg.Wait()

	return err
}

func CopyFile(source, destination string) error {
	srcFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile) // first var shows number of bytes
	if err != nil {
		return err
	}

	err = destFile.Sync()
	if err != nil {
		return err
	}

	return nil
}

func CopyFileEx(
	source string,
	destination string,
	fs fi.FileSystem,
) error {
	srcFh, err := fs.OpenFile(source, os.O_RDONLY, 0o644)
	if err != nil {
		return err
	}
	defer srcFh.Close()

	if _, ok := srcFh.(io.Writer); ok {
		// fs.OpenFile return a io.Writer, so use io.Copy
		dstFh, err := fs.OpenFile(destination, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer dstFh.Close()
		if dstW, ok := dstFh.(io.Writer); ok {
			if _, err := io.Copy(dstW, srcFh); err != nil {
				return err
			}
			if syncW, ok := dstFh.(fi.SyncFileSystem); ok {
				return syncW.Sync()
			}
		} else {
			return errors.Errorf("Write error: file is not a io.Writer")
		}
	} else {
		// use WriteFile to write
		// ! WARN: io.ReadAll will read all file content into memory
		bytes, err := io.ReadAll(srcFh)
		err = fs.WriteFile(destination, bytes, 0o644)
		if err != nil {
			return err
		}
	}

	return nil
}

func SaveFile(raw interface{}, filePath string) error {
	fp, err := os.Create(filePath)
	switch v := raw.(type) {
	case []byte:
		_, err = io.Copy(fp, bytes.NewReader(v))
	case *gzip.Reader:
		_, err = io.Copy(fp, v)
	default:
		return errors.Errorf("Type does not match.")
	}
	if err != nil {
		return errors.Errorf("Write file error: %s", err)
	}
	return nil
}

func GetAllFiles(path string) (fileNames []string, err error) {
	rd, err := ioutil.ReadDir(path)
	for _, fi := range rd {
		if !fi.IsDir() {
			fileNames = append(fileNames, fi.Name())
		}
	}
	return
}

func GetFileModTime(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		log.Println("open file error")
		return time.Now().Unix()
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		log.Println("stat fileinfo error")
		return time.Now().Unix()
	}

	return fi.ModTime().Unix()
}

func GetLatestFile(dir, suffix string) (filename string, err error) {
	if dir == "" {
		dir = "."
	}
	fileNames, err := GetAllFiles(dir)
	if err != nil {
		return "", errors.Errorf("cannot fetch files in dir(%s): %s", dir, err)
	}
	fileTimes := []int{}
	fileTimesMap := map[int]string{}
	for _, fileName := range fileNames {
		if strings.HasSuffix(fileName, suffix) {
			fileTime := int(GetFileModTime(path.Join(dir, fileName)))
			fileTimes = append(fileTimes, fileTime)
			fileTimesMap[fileTime] = fileName
		}
	}
	if len(fileTimes) == 0 {
		return "", errors.Errorf("cannot find file in %s", dir)
	}
	sort.Ints(fileTimes)
	return fileTimesMap[fileTimes[len(fileTimes)-1]], nil
}

// GetFileSha256 计算文件的SHA256值
func GetFileSha256(filepath string) string {
	var f *os.File
	var err error
	var sha256Value string = ""
	if _, err = os.Stat(filepath); err != nil {
		return ""
	}

	if f, err = os.Open(filepath); err != nil {
		return ""
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return ""
	}
	sha256Value = hex.EncodeToString(hasher.Sum(nil))
	return sha256Value
}

func GetFileMd5(filepath string) string {
	var f *os.File
	var err error
	var md5Value string = ""
	if _, err = os.Stat(filepath); err != nil {
		return ""
	}

	if f, err = os.Open(filepath); err != nil {
		return ""
	}

	md5h := md5.New()
	io.Copy(md5h, f)
	md5Value = hex.EncodeToString(md5h.Sum([]byte("")))
	f.Close()
	return md5Value
}

func CalcMd5(items ...interface{}) string {
	e := fmt.Sprintf("%v", items)
	md5Raw := md5.Sum([]byte(e))
	return hex.EncodeToString(md5Raw[:])
}

func CalcSha1(items ...interface{}) string {
	s := fmt.Sprintf("%v", items)
	raw := sha1.Sum([]byte(s))
	return hex.EncodeToString(raw[:])
}

func CalcSha256(items ...interface{}) string {
	s := fmt.Sprintf("%v", items)
	raw := sha256.Sum256([]byte(s))
	return hex.EncodeToString(raw[:])
}

func CalcSha1WithSuffix(items []interface{}, suffix string) string {
	s := fmt.Sprintf("%v", items) + suffix
	raw := sha1.Sum([]byte(s))
	return hex.EncodeToString(raw[:])
}

func GetFileAbsPath(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.Errorf(" empty file path")
	}

	absfilename, err := filepath.Abs(filepath.Dir(filePath))
	if err != nil {
		return "", err
	}
	absfilename = path.Join(absfilename, filepath.Base(filePath))
	return absfilename, nil
}

func GetFileAbsDir(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.Errorf(" empty file path")
	}

	absfilename, err := filepath.Abs(filepath.Dir(filePath))
	if err != nil {
		return "", err
	}
	return absfilename, nil
}

func ConvertTextFileToYakFuzztagByPath(file_bin_path string) (string, error) {
	var ret string
	file, err := os.Open(file_bin_path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	var fuzztagContentArr []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fuzztagContentArr = append(fuzztagContentArr, line)
	}
	fuzztagContent := strings.Join(fuzztagContentArr, "|")
	ret = fmt.Sprintf("{{array(%s)}}", fuzztagContent)
	return ret, nil
}

func SaveTempFile(content interface{}, pattern string) (string, error) {
	contentString := InterfaceToString(content)
	fp, err := os.CreateTemp(os.TempDir(), pattern)
	if err != nil {
		return "", err
	}
	fp.WriteString(contentString)
	fp.Close()
	return fp.Name(), nil
}

func IsSubPath(sub, parent string) bool {
	up := ".." + string(os.PathSeparator)
	parent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	sub, err = filepath.Abs(sub)
	if err != nil {
		return false
	}
	// path-comparisons using filepath.Abs don't work reliably according to docs (no unique representation).
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(rel, up) && // ../
		rel != ".." && // `sub` is `parent` parent path
		rel != "." { // same path
		return true
	}
	return false
}

// FileExists checks if a file exists and is not a directory.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

type CRLFtoLFReader struct {
	// source 是底层的原始 reader
	source io.Reader
	// sawCR 用于记录上一次 Read 操作的最后一个字节是否是 '\r'
	sawCR bool
}

// NewCRLFtoLFReader 是 CRLFtoLFReader 的构造函数。
func NewCRLFtoLFReader(source io.Reader) *CRLFtoLFReader {
	return &CRLFtoLFReader{source: source}
}

// Read 实现了 io.Reader 接口。
// 这是实现转换逻辑的核心。
func (r *CRLFtoLFReader) Read(p []byte) (n int, err error) {
	rawN, rawErr := r.source.Read(p)
	if rawN == 0 {
		return 0, rawErr
	}

	if r.sawCR && p[0] == '\n' {
		copy(p, p[1:rawN])
		rawN--
		if rawN == 0 {
			return 0, rawErr
		}
	}
	r.sawCR = false

	writePos := 0
	for readPos := 0; readPos < rawN; readPos++ {
		if p[readPos] == '\r' {
			if readPos+1 == rawN {
				r.sawCR = true // 设置状态，留到下一次 Read 时处理
				continue       // 跳过这个 \r，不把它写入
			}

			// 如果 \r 后面跟着 \n
			if p[readPos+1] == '\n' {
				continue // 同样跳过这个 \r，\n 会在下一次循环中被正常写入
			}
		}

		p[writePos] = p[readPos]
		writePos++
	}

	return writePos, rawErr
}
