package utils

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	var (
		existedFile string
	)
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
	var (
		existedFile string
	)
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

	if stats.Mode()&0111 == 0 {
		return ""
	}

	return r
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
