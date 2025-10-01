package ziputil

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

// 压缩文件
// files 文件数组，可以是不同dir下的文件或者文件夹
// dest 压缩文件存放地址
func Compress(files []*os.File, dest string) error {
	d, _ := os.Create(dest)
	defer d.Close()
	w := zip.NewWriter(d)
	defer w.Close()
	for _, file := range files {
		err := compress(file, "", w)
		if err != nil {
			return err
		}
	}
	return nil
}

func CompressByName(files []string, dest string) error {
	if utils.GetFirstExistedPath(dest) != "" {
		return utils.Errorf("dest[%s] is existed", dest)
	}

	d, err := os.Create(dest)
	if err != nil {
		return utils.Errorf("create zip dest:%v failed: %s", dest, err)
	}
	defer d.Close()

	zipWriter := zip.NewWriter(d)
	defer zipWriter.Close()

	compressFile := func(filename string) error {
		log.Infof("zip compress %s", filename)
		fileFp, err := os.Open(filename)
		if err != nil {
			return utils.Errorf("cannot open[%s] as fp: %s", filename, err)
		}
		defer fileFp.Close()

		state, err := fileFp.Stat()
		if err != nil {
			return utils.Errorf("cannot fetch filefp stats: %s", err)
		}

		header, err := zip.FileInfoHeader(state)
		if err != nil {
			return utils.Errorf("file info reader create failed: %s", err)
		}
		header.Name = path.Join(".", filename)
		fpWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return utils.Errorf("zip create header failed: %s", err)
		}

		if !state.IsDir() {
			_, err = io.Copy(fpWriter, fileFp)
			if err != nil {
				return utils.Errorf("compress file content failed: %s", err)
			}
		}
		return nil
	}
	for _, filename := range files {
		if strings.Contains(filename, "..") {
			return utils.Errorf("filename[%s] invalid: %s", filename, "cannot contains `..`")
		}
		if utils.IsFile(filename) {
			err := compressFile(filename)
			if err != nil {
				return err
			}
		}

		if utils.IsDir(filename) {
			absPathName, _ := filepath.Abs(filename)
			if absPathName == "" {
				absPathName = filename
			}
			absPath := filepath.IsAbs(filename)
			var baseDir = filename
			if absPath {
				baseDir = absPathName
			} else {
				baseDir = filename
			}

			infos, err := utils.ReadFilesRecursively(baseDir)
			if err != nil {
				return utils.Errorf("read dirs[%s] failed: %s", baseDir, err)
			}

			for _, info := range infos {
				rPath := info.Path
				if !absPath {
					if strings.HasPrefix(rPath, absPathName) {
						rPath = rPath[len(absPathName):]
						rPath = filepath.Join(filename, rPath)
					}
				}

				log.Infof("found file(dir: %v): %s[%v]", info.IsDir, info.Name, rPath)
				err := compressFile(rPath)
				if err != nil {
					return utils.Errorf("compress path[%s] in dir[%s] failed: %s", rPath, absPathName, err)
				}
			}
			continue
		}

	}
	return nil
}

func compress(file *os.File, prefix string, zw *zip.Writer) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		prefix = prefix + "/" + info.Name()
		fileInfos, err := file.Readdir(-1)
		if err != nil {
			return err
		}
		for _, fi := range fileInfos {
			f, err := os.Open(file.Name() + "/" + fi.Name())
			if err != nil {
				return err
			}
			err = compress(f, prefix, zw)
			if err != nil {
				return err
			}
		}
	} else {
		header, err := zip.FileInfoHeader(info)
		header.Name = prefix + "/" + header.Name
		if err != nil {
			return err
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, file)
		file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func DeCompressFromRaw(raw []byte, dest string) error {
	absDestFull, err := filepath.Abs(dest)
	if err != nil {
		return utils.Errorf("cannot found dest(%s) abspath: %s", dest, err)
	}
	_ = os.MkdirAll(absDestFull, 0777)

	size := len(raw)
	mfile := memfile.New(raw)
	reader, err := zip.NewReader(mfile, int64(size))
	if err != nil {
		return utils.Errorf("create zip reader failed: %s", err)
	}
	for _, file := range reader.File {
		filename := filepath.Join(dest, file.Name)
		filenameAbs, err := filepath.Abs(filename)
		if err != nil {
			return utils.Errorf("cannot convert %s as abs path: %s", filename, err)
		}
		if !strings.HasPrefix(filenameAbs, absDestFull) {
			return utils.Errorf("extract file(%s) [abs:%s] is not in [%s]", filename, filenameAbs, absDestFull)
		}
		if file.FileInfo().IsDir() {
			err := os.MkdirAll(filename, 0777)
			if err != nil {
				return utils.Errorf("mkdir failed: %s", err)
			}
			continue
		}

		dirName := filepath.Dir(filename)
		err = os.MkdirAll(dirName, 0777)
		if err != nil {
			log.Errorf("mkdir [%s] failed: %s", dirName, err)
			return err
		}

		// 打开需要解压的文件
		rc, err := file.Open()
		if err != nil {
			return err
		}

		w, err := os.Create(filename)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(w, rc)
		w.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// 解压
func DeCompress(zipFile, dest string) error {
	raw, err := ioutil.ReadFile(zipFile)
	if err != nil {
		return err
	}
	return DeCompressFromRaw(raw, dest)
}
