package yaklib

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/hpcloud/tail"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mfreader"
)

type _yakFile struct {
	file *os.File
}

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
			_, _ = file.WriteString(fmt.Sprintf("%v\n", line))
		}
	default:
		return utils.Errorf("not support type: %v", reflect.TypeOf(ret))
	}
	return nil
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

func (y *_yakFile) ReadLines() []string {
	sc := bufio.NewScanner(y.file)
	sc.Split(bufio.ScanLines)

	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func _fileIsLink(file string) bool {
	if _, err := os.Readlink(file); err != nil {
		return false
	}
	return true
}

func _fileIsDir(file string) bool {
	if info, err := os.Stat(file); err != nil {
		return false
	} else {
		if info.IsDir() {
			return true
		}
		return false
	}
}

func _tempFile(dirPart ...string) (*_yakFile, error) {
	var dir = consts.GetDefaultYakitBaseTempDir()
	if len(dirPart) > 0 {
		dir = filepath.Join(dirPart...)
	}
	f, err := ioutil.TempFile(dir, "yak-*.tmp")
	if err != nil {
		return nil, err
	}
	return &_yakFile{file: f}, nil
}

func readLines(i interface{}) []string {
	f := utils.InterfaceToString(i)
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return make([]string, 0)
	}
	return utils.ParseStringToLines(string(c))
}

var FileExport = map[string]interface{}{
	"ReadLines":  readLines,
	"GetDirPath": filepath.Dir,
	"Split":      filepath.Split,
	"IsExisted": func(name string) bool {
		ret, _ := utils.PathExists(name)
		return ret
	},
	"IsFile": func(file string) bool {
		if info, err := os.Stat(file); err != nil {
			return false
		} else {
			if info.IsDir() {
				return false
			}

			return true
		}
	},
	"IsAbs":  filepath.IsAbs,
	"IsLink": _fileIsLink,
	"IsDir":  _fileIsDir,
	"Join":   filepath.Join,

	// flags
	"O_RDWR":   os.O_RDWR,
	"O_CREATE": os.O_CREATE,
	"O_APPEND": os.O_APPEND,
	"O_EXCL":   os.O_EXCL,
	"O_RDONLY": os.O_RDONLY,
	"O_SYNC":   os.O_SYNC,
	"O_TRUNC":  os.O_TRUNC,
	"O_WRONLY": os.O_WRONLY,

	// 文件打开
	"ReadAll":  ioutil.ReadAll,
	"ReadFile": ioutil.ReadFile,
	"TempFile": _tempFile,
	"TempFileName": func() (string, error) {
		f, err := _tempFile()
		if err != nil {
			return "", err
		}
		f.Close()
		return f.Name(), nil
	},
	"Mkdir": func(name string) error {
		return os.Mkdir(name, os.ModePerm)
	},
	"MkdirAll": func(name string) error {
		return os.MkdirAll(name, os.ModePerm)
	},
	"Rename": os.Rename,
	"Remove": os.RemoveAll,
	"Create": func(name string) (*_yakFile, error) {
		f, err := os.Create(name)
		if err != nil {
			return nil, err
		}
		return &_yakFile{file: f}, nil
	},

	// 打开文件操作
	"Open": func(name string) (*_yakFile, error) {
		file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, os.ModePerm)
		if err != nil {
			return nil, err
		}
		return &_yakFile{file: file}, nil
	},
	"OpenFile": func(name string, flag int, perm os.FileMode) (*_yakFile, error) {
		f, err := os.OpenFile(name, flag, perm)
		if err != nil {
			return nil, err
		}
		return &_yakFile{file: f}, nil
	},
	"Stat":  os.Stat,
	"Lstat": os.Lstat,
	"Save":  _saveFile,
	"SaveJson": func(name string, i interface{}) error {
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
	},

	// 模仿 Linux 命令的一些函数
	// 自定义的好用 API
	"Cat": func(i string) {
		raw, err := ioutil.ReadFile(i)
		_diewith(err)
		fmt.Print(string(raw))
	},
	"TailF": func(i string, line func(i string)) {
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
	},
	"Mv":  os.Rename,
	"Rm":  os.RemoveAll,
	"Cp":  _fileCopy,
	"Dir": _lsDir,
	"Ls":  _lsDir,
	//"DeepLs": _lsDirAll,
	"Abs": func(i string) string {
		raw, err := filepath.Abs(i)
		if err != nil {
			log.Errorf("fetch abs path failed for[%v]: %s", i, raw)
			return i
		}
		return raw
	},
	"ReadFileInfoInDirectory": utils.ReadFilesRecursively,
	"ReadDirInfoInDirectory":  utils.ReadDirsRecursively,
	"NewMultiFileLineReader":  mfreader.NewMultiFileLineReader,
}

func _lsDirAll(i string) []*utils.FileInfo {
	raw, err := utils.ReadDirsRecursively(i)
	if err != nil {
		log.Errorf("dir %v failed: %s", i, err)
		return nil
	}
	return raw
}

// Copy the src file to dst. Any existing file will be overwritten and will not
// copy file attributes.
func _fileCopy(src, dst string) error {
	if _fileIsDir(src) {
		return utils.Errorf("SRC:%v is a dir", src)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func _lsDir(i string) []*utils.FileInfo {
	raw, err := utils.ReadDir(i)
	if err != nil {
		log.Errorf("dir %v failed: %s", i, err)
		return nil
	}
	return raw
}
