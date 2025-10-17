package yaklib

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/mimetype"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/hpcloud/tail"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mfreader"
)

var SEP = string(filepath.Separator)

type _yakFile struct {
	file *os.File
	rw   *bufio.ReadWriter
}

// Save 将字符串或字节切片或字符串切片写入到文件中，如果文件不存在则创建，如果文件存在则覆盖，返回错误
// Example:
// ```
// file.Save("/tmp/test.txt", "hello yak")
// ```
func _saveFile(fileName string, i interface{}) error {
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	switch ret := i.(type) {
	case string:
		_, err = file.WriteString(ret)
		if err != nil {
			return err
		}
	case []byte:
		_, err = file.Write(ret)
		if err != nil {
			return err
		}
	case []string:
		for _, line := range ret {
			_, err = file.WriteString(fmt.Sprintf("%v\n", line))
			if err != nil {
				return err
			}
		}
	default:
		return utils.Errorf("not support type: %v", reflect.TypeOf(ret))
	}
	return nil
}

// SaveJson 将字符串或字节切片或字符串切片写入到文件中，如果文件不存在则创建，如果文件存在则覆盖，返回错误
// 与 Save 不同的是，如果传入的参数是其他类型，会尝试将其序列化为 json 字符再写入到文件中
// Example:
// ```
// file.SaveJson("/tmp/test.txt", "hello yak")
// ```
func _saveJson(name string, i interface{}) error {
	switch ret := i.(type) {
	case []byte:
		return _saveFile(name, ret)
	case string:
		return _saveFile(name, ret)
	case []string:
		return _saveFile(name, ret)
	default:
		raw, err := json.Marshal(i)
		if err != nil {
			return utils.Errorf("marshal %v failed: %s", spew.Sdump(i), err)
		}
		return _saveFile(name, raw)
	}
}

func (y *_yakFile) WriteLine(i interface{}) (int, error) {
	switch ret := i.(type) {
	case string:
		return y.file.WriteString(fmt.Sprintf("%v\n", ret))
	case []byte:
		return y.file.Write([]byte(fmt.Sprintf("%v\n", string(ret))))
	case []string:
		var res int
		for _, line := range ret {
			line = strings.TrimRight(line, " \t\n\r")
			n := len(line)
			if n == 0 {
				continue
			}
			var err error
			n, err = y.file.WriteString(line + "\n")
			if err != nil {
				log.Error(err)
			}
			res += n
		}
		return res, nil
	default:
		raw, err := json.Marshal(i)
		if err != nil {
			return 0, err
		}
		raw = append(raw, '\n')
		return y.WriteLine(raw)
	}
}

func (y *_yakFile) WriteString(i string) (int, error) {
	return y.file.WriteString(i)
}

func (y *_yakFile) Write(i interface{}) (int, error) {
	switch ret := i.(type) {
	case string:
		return y.WriteString(ret)
	case []byte:
		return y.file.Write(ret)
	default:
		raw, err := json.Marshal(i)
		if err != nil {
			return 0, err
		}
		return y.WriteLine(raw)
	}
}

func (y *_yakFile) GetOsFile() *os.File {
	return y.file
}

// Seek 移动文件指针，返回新的偏移量和错误
func (y *_yakFile) Seek(offset int64, whence int) (int64, error) {
	return y.file.Seek(offset, whence)
}

func (y *_yakFile) Truncated(size int64) error {
	return y.file.Truncate(size)
}

func (y *_yakFile) Sync() error {
	return y.file.Sync()
}

func (y *_yakFile) Name() string {
	return y.file.Name()
}

func (y *_yakFile) Close() error {
	return y.file.Close()
}

func (y *_yakFile) Read(b []byte) (int, error) {
	return y.file.Read(b)
}

func (y *_yakFile) ReadAt(b []byte, off int64) (int, error) {
	return y.file.ReadAt(b, off)
}

func (y *_yakFile) ReadAll() ([]byte, error) {
	return ioutil.ReadAll(y.file)
}

func (y *_yakFile) ReadString() (string, error) {
	raw, err := y.ReadAll()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (y *_yakFile) ReadLine() (string, error) {
	return utils.BufioReadLineString(y.rw.Reader)
}

func (y *_yakFile) ReadLines() []string {
	lines := make([]string, 0)
	for {
		line, err := utils.BufioReadLineString(y.rw.Reader)
		if err != nil {
			break
		}
		lines = append(lines, line)
	}
	return lines
}

// IsLink 判断文件是否是一个符号链接
// Example:
// ```
// 假设 /usr/bin/bash 是一个符号链接，指向 /bin/bash
// file.IsLink("/usr/bin/bash") // true
// file.IsLink("/bin/bash") // false
// ```
func _fileIsLink(file string) bool {
	if _, err := os.Readlink(file); err != nil {
		return false
	}
	return true
}

// TempFile 创建一个临时文件，返回一个文件结构体引用与错误
// Example:
// ```
// f, err = file.TempFile()
// die(err)
// defer f.Close()
// f.WriteString("hello yak")
// ```
func _tempFile(dirPart ...string) (*_yakFile, error) {
	return _tempFileEx("yak-*.tmp", dirPart...)
}

func _tempFileEx(pattern string, dirPart ...string) (*_yakFile, error) {
	dir := consts.GetDefaultYakitBaseTempDir()
	if len(dirPart) > 0 {
		dir = filepath.Join(dirPart...)
	}
	f, err := ioutil.TempFile(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &_yakFile{file: f}, nil
}

// TempFileName 创建一个临时文件，返回一个文件名与错误
// Example:
// ```
// name, err = file.TempFileName()
// die(err)
// defer os.Remove(name)
// file.Save(name, "hello yak")
//
// name, err = file.TempFileName("pattern-*.txt")
//
//	if die(err) {
//		return
//	}
//
// defer os.Remove(name)
// file.Save(name, "hello yak")
// ```
func _tempFileName(pattern ...string) (string, error) {
	if len(pattern) <= 0 {
		f, err := _tempFile()
		if err != nil {
			return "", err
		}
		f.Close()
		return f.Name(), nil
	}

	f, err := _tempFileEx(strings.Join(pattern, "-"))
	if err != nil {
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// Mkdir 创建一个目录，返回错误
// Example:
// ```
// err = file.Mkdir("/tmp/test")
// ```
func _mkdir(name string) error {
	return os.Mkdir(name, os.ModePerm)
}

// MkdirAll 创建一个递归创建一个目录，返回错误
// Example:
// ```
// // 假设存在 /tmp 目录，不存在 /tmp/test 目录
// err = file.MkdirAll("/tmp/test/test2")
// ```
func _mkdirAll(name string) error {
	return os.MkdirAll(name, os.ModePerm)
}

// Rename 重命名一个文件或文件夹，返回错误，这个函数也会移动文件或文件夹
// ! 在 windows 下，无法将文件移动到不同的磁盘
// Example:
// ```
// // 假设存在 /tmp/test.txt 文件
// err = file.Rename("/tmp/test.txt", "/tmp/test2.txt")
// ```
func _rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// Mv 重命名一个文件或文件夹，返回错误，这个函数也会移动文件或文件夹，它是 Rename 的别名
// ! 在 windows 下，无法将文件移动到不同的磁盘
// Example:
// ```
// // 假设存在 /tmp/test.txt 文件
// err = file.Rename("/tmp/test.txt", "/tmp/test2.txt")
// ```
func _mv(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// Remove 删除路径及其包含的所有子路径
// Example:
// ```
// // 假设存在 /tmp/test/test.txt 文件和 /tmp/test/test2.txt 文件
// err = file.Remove("/tmp/test")
// ```
func _remove(path string) error {
	return os.RemoveAll(path)
}

// Rm 删除路径及其包含的所有子路径，它是 Remove 的别名
// Example:
// ```
// // 假设存在 /tmp/test/test.txt 文件和 /tmp/test/test2.txt 文件
// err = file.Remove("/tmp/test")
// ```
func _rm(path string) error {
	return os.RemoveAll(path)
}

// Create 创建一个文件，返回一个文件结构体引用与错误
// Example:
// ```
// f, err = file.Create("/tmp/test.txt")
// ```
func _create(name string) (*_yakFile, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	return &_yakFile{file: f}, nil
}

// ReadLines 尝试读取一个文件中的所有行，返回一个字符串切片，会去除BOM头和空行
// Example:
// ```
// lines = file.ReadLines("/tmp/test.txt")
// ```
func _fileReadLines(i interface{}) []string {
	f := utils.InterfaceToString(i)
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return make([]string, 0)
	}
	return utils.ParseStringToLines(string(c))
}

// ReadLinesWithCallback 尝试读取一个文件中的所有行，每读取一行，便会调用回调函数，返回错误
// Example:
// ```
// err = file.ReadLinesWithCallback("/tmp/test.txt", func(line) { println(line) })
// ```
func _fileReadLinesWithCallback(i interface{}, callback func(string)) error {
	filename := utils.InterfaceToString(i)
	f, err := _fileOpenWithPerm(filename, os.O_RDONLY, 0o644)
	if err != nil {
		return err
	}
	for {
		line, err := f.ReadLine()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		callback(line)
	}
	return nil
}

// GetDirPath 返回路径中除最后一个元素之后的路径，这通常是原本路径的目录
// Example:
// ```
// file.GetDirPath("/usr/bin/bash") // "/usr/bin/"
// ```
func _fileGetDirPath(path string) string {
	dirPath := filepath.Dir(path)
	if dirPath == "" {
		return dirPath
	}
	if strings.HasSuffix(dirPath, SEP) {
		return dirPath
	} else {
		return dirPath + SEP
	}
}

// Split 以操作系统的默认路径分隔符分割路径，返回目录和文件名
// Example:
// ```
// file.Split("/usr/bin/bash") // "/usr/bin", "bash"
// ```
func _filePathSplit(path string) (string, string) {
	return filepath.Split(path)
}

// IsExisted 判断文件或目录是否存在
// Example:
// ```
// file.IsExisted("/usr/bin/bash")
// ```
func _fileIsExisted(path string) bool {
	ret, _ := utils.PathExists(path)
	return ret
}

// IsFile 判断路径是否存在且是一个文件
// Example:
// ```
// // 假设存在 /usr/bin/bash 文件
// file.IsFile("/usr/bin/bash") // true
// file.IsFile("/usr/bin") // false
// ```
func _fileIsFile(path string) bool {
	return utils.IsFile(path)
}

// IsDir 判断路径是否存在且是一个目录
// Example:
// ```
// // 假设存在 /usr/bin/bash 文件
// file.IsDir("/usr/bin") // true
// file.IsDir("/usr/bin/bash") // false
// ```
func _fileIsDir(path string) bool {
	return utils.IsDir(path)
}

// IsAbs 判断路径是否是绝对路径
// Example:
// ```
// file.IsAbs("/usr/bin/bash") // true
// file.IsAbs("../../../usr/bin/bash") // false
// ```
func _fileIsAbs(path string) bool {
	return filepath.IsAbs(path)
}

// Join 将任意数量的路径以默认路径分隔符链接在一起
// Example:
// ```
// file.Join("/usr", "bin", "bash") // "/usr/bin/bash"
// ```
func _fileJoin(path ...string) string {
	return filepath.Join(path...)
}

// ReadAll 从 Reader 读取直到出现错误或 EOF，然后返回字节切片与错误
// Example:
// ```
// f, err = file.Open("/tmp/test.txt")
// content, err = file.ReadAll(f)
// ```
func _fileReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// ReadFile 读取一个文件的所有内容，返回字节切片与错误
// Example:
// ```
// content, err = file.ReadFile("/tmp/test.txt")
// ```
func _fileReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func _lsDirAll(i string) []*utils.FileInfo {
	raw, err := utils.ReadDirsRecursively(i)
	if err != nil {
		log.Errorf("dir %v failed: %s", i, err)
		return nil
	}
	return raw
}

// Cp 拷贝文件或目录，返回错误
// Example:
// ```
// file.Cp("/tmp/test.txt", "/tmp/test2.txt")
// file.Cp("/tmp/test", "/root/tmp/test")
// ```
func _fileCopy(src, dst string) error {
	return utils.CopyDirectory(src, dst, false)
}

// Ls 列出一个目录下的所有文件和目录，返回一个文件信息切片
// Example:
// ```
// for f in file.Ls("/tmp") {
// println(f.Name)
// }
// ```
func _ls(i string) []*utils.FileInfo {
	raw, err := utils.ReadDir(i)
	if err != nil {
		log.Errorf("dir %v failed: %s", i, err)
		return nil
	}
	return raw
}

// Dir 列出一个目录下的所有文件和目录，返回一个文件信息切片，它是 Ls 的别名
// Example:
// ```
// for f in file.Ls("/tmp") {
// println(f.Name)
// }
// ```
func _dir(i string) []*utils.FileInfo {
	raw, err := utils.ReadDir(i)
	if err != nil {
		log.Errorf("dir %v failed: %s", i, err)
		return nil
	}
	return raw
}

// Open 打开一个文件，返回一个文件结构体引用与错误
// Example:
// ```
// f, err = file.Open("/tmp/test.txt")
// content, err = file.ReadAll(f)
// ```
func _fileOpen(name string) (*_yakFile, error) {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return nil, err
	}
	return &_yakFile{file: file, rw: bufio.NewReadWriter(bufio.NewReader(file), bufio.NewWriter(file))}, nil
}

// OpenFile 打开一个文件，使用 file.O_CREATE ... 和权限控制，返回一个文件结构体引用与错误
// Example:
// ```
// f = file.OpenFile("/tmp/test.txt", file.O_CREATE|file.O_RDWR, 0o777)~; defer f.Close()
// ```
func _fileOpenWithPerm(name string, flags int, mode os.FileMode) (*_yakFile, error) {
	file, err := os.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return &_yakFile{file: file, rw: bufio.NewReadWriter(bufio.NewReader(file), bufio.NewWriter(file))}, nil
}

// Stat 返回一个文件的信息和错误
// Example:
// ```
// info, err = file.Stat("/tmp/test.txt")
// desc(info)
// ```
func _fileStat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// Lstat 返回一个文件的信息和错误，如果文件是一个符号链接，返回的是符号链接的信息
// Example:
// ```
// info, err = file.Lstat("/tmp/test.txt")
// desc(info)
// ```
func _fileLstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

// Cat 模拟 unix 命令 cat，打印文件内容到标准输出
// Example:
// ```
// file.Cat("/tmp/test.txt")
// ```
func _cat(i string) {
	raw, err := ioutil.ReadFile(i)
	_diewith(err)
	fmt.Print(string(raw))
}

// TailF 模拟 unix 命令 tail -f，执行这个函数会一直阻塞，打印文件内容到标准输出，如果文件有变化，会自动打印新的内容
// Example:
// ```
// file.TailF("/tmp/test.txt")
// ```
func _tailf(i string, line func(i string)) {
	t, err := tail.TailFile(i, tail.Config{
		MustExist: false,
		Follow:    true,
		Logger:    tail.DiscardingLogger,
	})
	if err != nil {
		log.Errorf("tail failed: %s", err)
		return
	}
	for {
		select {
		case l, ok := <-t.Lines:
			if !ok {
				return
			}
			if line != nil {
				line(l.Text)
			}
		}
	}
}

// Abs 返回一个路径的绝对路径
// Example:
// ```
// // 假设当前目录是 /tmp
// file.Abs("./test.txt") // /tmp/test.txt
// ```
func _fileAbs(i string) string {
	if runtime.GOOS != "windows" && strings.HasPrefix(i, "~") {
		if len(i) == 1 {
			return GetHomeDir()
		}
		if i[1] == '/' {
			after := i[2:]
			hdir := GetHomeDir()
			return filepath.Join(hdir, after)
		}
	}

	raw, err := filepath.Abs(i)
	if err != nil {
		log.Errorf("fetch abs path failed for[%v]: %s", i, raw)
		return i
	}
	return raw
}

// ReadFileInfoInDirectory 读取一个目录下的所有文件信息，返回一个文件信息切片和错误
// Example:
// ```
// for f in file.ReadFileInfoInDirectory("/tmp")~ {
// println(f.Name)
// }
// ```
func _readFileInfoInDirectory(path string) ([]*utils.FileInfo, error) {
	return utils.ReadFilesRecursively(path)
}

// ReadDirInfoInDirectory 读取一个目录下的所有目录信息，返回一个文件信息切片和错误
// Example:
// ```
// for d in file.ReadDirInfoInDirectory("/tmp")~ {
// println(d.Name)
// }
// ```
func _readDirInfoInDirectory(path string) ([]*utils.FileInfo, error) {
	return utils.ReadDirsRecursively(path)
}

// NewMultiFileLineReader 创建一个多文件读取器，返回一个多文件读取器结构体引用和错误
// Example:
// ```
// // 假设存在 /tmp/test.txt 文件，内容为 123
// // 假设存在 /tmp/test2.txt 文件，内容为 456
// m, err = file.NewMultiFileLineReader("/tmp/test.txt", "/tmp/test2.txt")
// for m.Next() {
// println(m.Text())
// }
// ```
func _newMultiFileLineReader(files ...string) (*mfreader.MultiFileLineReader, error) {
	return mfreader.NewMultiFileLineReader(files...)
}

// Walk 遍历一个目录下的所有文件和目录，返回错误
// Example:
// ```
// file.Walk("/tmp", func(info) {println(info.Name); return true})~
// ```
func _walk(uPath string, i func(info *utils.FileInfo) bool) error {
	return utils.ReadDirsRecursivelyCallback(uPath, i)
}

// GetExt 获取文件的扩展名
// Example:
// ```
// file.GetExt("/tmp/test.txt") // ".txt"
// ```
func _ext(s string) string {
	return filepath.Ext(s)
}

// GetBase 获取文件的基本名
// Example:
// ```
// file.GetBase("/tmp/test.txt") // "test.txt"
// ```
func _getBase(s string) string {
	return filepath.Base(s)
}

// Clean 清理路径中的多余的分隔符和 . 和 ..
// Example:
// ```
// file.Clean("/tmp/../tmp/test.txt") // "/tmp/test.txt"
// ```
func _clean(s string) string {
	return filepath.Clean(s)
}

var FileExport = map[string]interface{}{
	"ReadLines":             _fileReadLines,
	"ReadLinesWithCallback": _fileReadLinesWithCallback,
	"GetDirPath":            _fileGetDirPath,
	"GetExt":                _ext,
	"GetBase":               _getBase,
	"Clean":                 _clean,
	"Split":                 _filePathSplit,
	"IsExisted":             _fileIsExisted,
	"IsFile":                _fileIsFile,
	"IsDir":                 _fileIsDir,
	"IsAbs":                 _fileIsAbs,
	"IsLink":                _fileIsLink,
	"Join":                  _fileJoin,

	// flags
	"O_RDWR":    os.O_RDWR,
	"O_CREATE":  os.O_CREATE,
	"O_APPEND":  os.O_APPEND,
	"O_EXCL":    os.O_EXCL,
	"O_RDONLY":  os.O_RDONLY,
	"O_SYNC":    os.O_SYNC,
	"O_TRUNC":   os.O_TRUNC,
	"O_WRONLY":  os.O_WRONLY,
	"SEPARATOR": SEP,

	// 文件打开
	"ReadAll":      _fileReadAll,
	"ReadFile":     _fileReadFile,
	"TempFile":     _tempFile,
	"TempFileName": _tempFileName,
	"Mkdir":        _mkdir,
	"MkdirAll":     _mkdirAll,
	"Rename":       _rename,
	"Remove":       _remove,
	"Create":       _create,

	// 打开文件操作
	"Open":     _fileOpen,
	"OpenFile": _fileOpenWithPerm,
	"Stat":     _fileStat,
	"Lstat":    _fileLstat,
	"Save":     _saveFile,
	"SaveJson": _saveJson,

	// 模仿 Linux 命令的一些函数
	// 自定义的好用 API
	"Cat":   _cat,
	"TailF": _tailf,
	"Mv":    _mv,
	"Rm":    _rm,
	"Cp":    _fileCopy,
	"Dir":   _ls,
	"Ls":    _dir,
	//"DeepLs": _lsDirAll,
	"Abs":                     _fileAbs,
	"ReadFileInfoInDirectory": _readFileInfoInDirectory,
	"ReadDirInfoInDirectory":  _readDirInfoInDirectory,
	"NewMultiFileLineReader":  _newMultiFileLineReader,
	"Walk":                    _walk,

	"DetectMIMETypeFromRaw":  mimetype.Detect,
	"DetectMIMETypeFromFile": mimetype.DetectFile,
}
