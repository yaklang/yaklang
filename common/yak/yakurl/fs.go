package yakurl

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	regexp2 "github.com/VillanCh/go-pcre2-lite/regexp2"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type fileSystemAction struct {
	fs fi.FileSystem
}

func getFirstQueryValue(query url.Values, keys ...string) string {
	for _, key := range keys {
		if value := query.Get(key); value != "" {
			return value
		}
	}
	return ""
}

func getBoolQueryValue(query url.Values, keys ...string) bool {
	value := strings.TrimSpace(getFirstQueryValue(query, keys...))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func (f *fileSystemAction) searchFileContent(path string, matcher *regexp_utils.YakRegexpUtils) ([]*ypb.KVPair, error) {
	file, err := f.fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var matches []*ypb.KVPair
	for lineNumber := 1; ; lineNumber++ {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			matched, err := matcher.MatchString(line)
			if err != nil {
				return nil, utils.Wrapf(err, "cannot match line %d", lineNumber)
			}
			if matched {
				matches = append(matches, &ypb.KVPair{
					Key:   strconv.Itoa(lineNumber),
					Value: utils.EscapeInvalidUTF8Byte([]byte(line)),
				})
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, readErr
		}
	}
	return matches, nil
}

func (f *fileSystemAction) searchFileResources(
	originParam *ypb.YakURL,
	query url.Values,
	rootPath string,
	rootInfo fs.FileInfo,
) ([]*ypb.YakURLResource, error) {
	keyword := getFirstQueryValue(query, "keyword", "search", "pattern", "query")
	if keyword == "" {
		return nil, utils.Error("keyword is required")
	}

	pattern := keyword
	if !getBoolQueryValue(query, "regex", "regexp", "isRegex", "useRegex") {
		pattern = regexp.QuoteMeta(pattern)
	}
	var regexpOptions []regexp_utils.YakRegexpUtilsOption
	if getBoolQueryValue(query, "ignoreCase", "caseInsensitive", "ignore-case") {
		regexpOptions = append(regexpOptions, regexp_utils.WithRegexpOption(regexp2.IgnoreCase))
	}
	matcher := regexp_utils.NewYakRegexpUtils(pattern, regexpOptions...)
	if !matcher.CanUse() {
		return nil, utils.Error("invalid search regular expression")
	}

	var resources []*ypb.YakURLResource
	searchFile := func(path string, info fs.FileInfo) error {
		matches, err := f.searchFileContent(path, matcher)
		if err != nil {
			return utils.Wrapf(err, "cannot search file[%s]", path)
		}
		if len(matches) == 0 {
			return nil
		}

		resource := f.fileInfoToResource(originParam, query, info, path, true)
		resource.Extra = append(resource.Extra, matches...)
		resources = append(resources, resource)
		return nil
	}

	if !rootInfo.IsDir() {
		if err := searchFile(rootPath, rootInfo); err != nil {
			return nil, err
		}
		return resources, nil
	}

	global := getBoolQueryValue(query, "global")
	var searchDirectory func(string) error
	searchDirectory = func(directory string) error {
		entries, err := f.fs.ReadDir(directory)
		if err != nil {
			return utils.Wrapf(err, "cannot read dir[%s]", directory)
		}
		for _, entry := range entries {
			path := f.fs.Join(directory, entry.Name())
			if entry.IsDir() {
				if global {
					if err := searchDirectory(path); err != nil {
						return err
					}
				}
				continue
			}

			info, err := entry.Info()
			if err != nil {
				return utils.Wrapf(err, "cannot stat path[%s]", path)
			}
			if err := searchFile(path, info); err != nil {
				return err
			}
		}
		return nil
	}
	if err := searchDirectory(rootPath); err != nil {
		return nil, err
	}
	return resources, nil
}

func (f *fileSystemAction) fileInfoToResource(originParam *ypb.YakURL, query url.Values, info fs.FileInfo, currentPath string, inDir bool) *ypb.YakURLResource {
	fs := f.fs
	yakURL := &ypb.YakURL{
		Schema:   originParam.Schema,
		User:     originParam.GetUser(),
		Pass:     originParam.GetPass(),
		Location: originParam.GetLocation(),
		Path:     currentPath,
		Query:    originParam.GetQuery(),
	}
	if !inDir {
		yakURL.Path = originParam.GetPath()
	}

	src := &ypb.YakURLResource{
		Size:              info.Size(),
		SizeVerbose:       utils.ByteSize(uint64(info.Size())),
		ModifiedTimestamp: info.ModTime().Unix(),
		Path:              currentPath,
		YakURLVerbose:     "",
		Url:               yakURL,
	}
	if info.IsDir() {
		src.ResourceType = "dir"
		src.VerboseType = "filesystem-directory"
		infos, err := fs.ReadDir(currentPath)
		if err == nil {
			src.HaveChildrenNodes = len(infos) > 0
		}
	}

	dirName, fileName := fs.PathSplit(currentPath)
	src.ResourceName = fileName
	if !info.IsDir() && info.Size() > 0 {
		src.VerboseName = fmt.Sprintf("%v [%v]", fileName, src.SizeVerbose)
	} else {
		src.VerboseName = fileName
	}
	src.Extra = append(src.Extra, &ypb.KVPair{
		Key:   "Directory-Name",
		Value: utils.EscapeInvalidUTF8Byte([]byte(dirName)),
	})
	if m := fs.ExtraInfo(currentPath); m != nil {
		for k, v := range m {
			src.Extra = append(src.Extra, &ypb.KVPair{
				Key:   k,
				Value: codec.AnyToString(v),
			})
		}
	}
	if !info.IsDir() && strings.ToLower(query.Get("detectPlainText")) == "true" {
		// read first 514 bytes to check if it is a plain text file
		fh, err := fs.Open(currentPath)
		if err == nil {
			defer fh.Close()
			size := src.Size
			if size > 514 {
				size = 514
			}

			buf := make([]byte, size)
			n, err := fh.Read(buf)
			if err == nil {
				buf = buf[:n]
				src.Extra = append(src.Extra, &ypb.KVPair{
					Key:   "IsPlainText",
					Value: utils.AsDebugString(utils.IsPlainText(buf)),
				})
			}
		}
	}
	return src
}

func (f fileSystemAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// available query:
	// op=list # list directory
	// op=search&keyword=xxx # search file content
	// global=true # recursively search subdirectories
	// regex=true # treat keyword as a Go regular expression
	// ignoreCase=true # case-insensitive matching
	// search result Extra: Key is the one-based line number, Value is the complete line
	// detectPlainText=true # detect if file is plain text, return: IsPlainText:true/false
	u := params.GetUrl()
	fs := f.fs
	absPath, _, _, err := f.FormatPath(params)
	if err != nil {
		return nil, err
	}

	info, err := fs.Stat(absPath)
	if err != nil {
		return nil, utils.Wrapf(err, "cannot stat path[%s]", u.GetPath())
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	var res []*ypb.YakURLResource
	switch query.Get("op") {
	case "list":
		if info.IsDir() {
			infos, err := fs.ReadDir(absPath)
			if err != nil {
				return nil, utils.Wrapf(err, "cannot read dir[%s]", u.GetPath())
			}
			for _, i := range infos {
				info, _ := i.Info()
				if info == nil {
					continue
				}
				currentPath := fs.Join(params.GetUrl().Path, info.Name())
				res = append(res, f.fileInfoToResource(params.GetUrl(), query, info, currentPath, true))
			}
		}
	case "search":
		res, err = f.searchFileResources(params.GetUrl(), query, absPath, info)
		if err != nil {
			return nil, err
		}
	default:
		res = append(res, f.fileInfoToResource(params.GetUrl(), query, info, absPath, false))
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func (f fileSystemAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	fs := f.fs

	absPath, dirname, _, err := f.FormatPath(params)
	if err != nil {
		return nil, err
	}

	info, err := fs.Stat(absPath)
	if err != nil {
		return nil, utils.Wrapf(err, "cannot stat path[%s]", u.GetPath())
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	switch query.Get("op") {
	case "rename":
		newName := query.Get("newname")
		if newName == "" {
			return nil, utils.Errorf("newname is required")
		}

		if !fs.IsAbs(newName) { // Compatible abs path and relative path
			newName = fs.Join(dirname, newName)
		}

		if ok, err := fs.Exists(newName); ok {
			return nil, utils.Errorf("file or directory is exists: %s", newName)
		} else if err != nil {
			return nil, utils.Errorf("cannot check file or directory exists: %s", newName)
		}

		err := fs.Rename(absPath, newName)
		if err != nil {
			relOldPath, relErr := fs.Rel(dirname, absPath)
			relNewPath, relErr2 := fs.Rel(dirname, newName)
			if relErr == nil && relErr2 == nil {
				return nil, utils.Errorf("cannot rename %s to %s", relOldPath, relNewPath)
			} else {
				return nil, utils.Errorf("cannot rename %s to %s", absPath, newName)
			}
		}

		absPath = newName
	case "content":
		fallthrough
	default:
		if info.IsDir() {
			return nil, utils.Errorf("cannot post to a directory: %s", u.GetPath())
		}
		err = fs.WriteFile(absPath, params.GetBody(), 0o644)
		if err != nil {
			return nil, utils.Wrapf(err, "cannot write file[%s]", u.GetPath())
		}
	}

	CheckUpdateFileMonitors(absPath)

	currentInfo, err := fs.Stat(absPath)
	if err != nil {
		return nil, utils.Wrapf(err, "cannot stat path[%s]", u.GetPath())
	}
	res := f.fileInfoToResource(params.GetUrl(), query, currentInfo, absPath, false)
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     1,
		Resources: []*ypb.YakURLResource{res},
	}, nil
}

func (f fileSystemAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// available query:
	// type=dir # create directory
	// type=file # create file
	// paste=true # paste, auto rename if file exist
	u := params.GetUrl()
	fs := f.fs

	absPath, _, _, err := f.FormatPath(params)
	if err != nil {
		return nil, err
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	isPaste := strings.ToLower(query.Get("paste")) == "true"

	pasteFixPath := func(absPath string) string {
		newAbsPath := absPath
		dirPath, name := fs.PathSplit(absPath)
		filenameWithoutExt, ext := name, fs.Ext(name)
		if len(ext) > 0 {
			filenameWithoutExt = strings.TrimSuffix(name, ext)
		}
		i := 1
		for {
			newName := fmt.Sprintf("%s(%d)%s", filenameWithoutExt, i, ext)
			newAbsPath = fs.Join(dirPath, newName)
			if found, _ := fs.Exists(newAbsPath); !found {
				break
			}
			i++
		}
		return newAbsPath
	}

	switch query.Get("type") {
	case "dir":
		if !isPaste {
			exists, err := fs.Exists(absPath)
			if exists {
				return nil, utils.Error("path exists")
			}
			if err != nil {
				return nil, utils.Wrap(err, "file exists check error")
			}
			if err := fs.MkdirAll(absPath, 0o755); err != nil {
				return nil, utils.Wrap(err, "cannot create directory")
			}
		} else {
			sourcePath := query.Get("src")
			if sourcePath == "" {
				return nil, utils.Error("source path is required")
			}
			sourceAbsPath, err := f.FormatPathRaw(sourcePath)
			if err != nil {
				return nil, err
			}
			exists, err := fs.Exists(sourcePath)
			if err != nil {
				return nil, utils.Wrap(err, "cannot check source path exists")
			}
			if !exists {
				return nil, utils.Errorf("source path not exists: %s", sourcePath)
			}

			_, sourceName := fs.PathSplit(sourceAbsPath)
			dstAbsPath := pasteFixPath(fs.Join(absPath, sourceName))
			err = utils.CopyDirectoryEx(
				sourceAbsPath, dstAbsPath, false, fs)
			if err != nil {
				return nil, utils.Wrap(err, "cannot paste directory")
			}
		}
	case "file":
		fallthrough
	default:
		exists, err := fs.Exists(absPath)
		if exists {
			if !isPaste {
				return nil, utils.Error("path exists") //  if path exists can't use put
			}
			absPath = pasteFixPath(absPath)
		} else if err != nil {
			return nil, utils.Wrap(err, "file exists check error")
		}

		err = fs.WriteFile(absPath, params.GetBody(), 0o644)
		if err != nil {
			return nil, utils.Wrapf(err, "cannot write file[%s]", u.GetPath())
		}
	}
	CheckUpdateFileMonitors(absPath)
	currentInfo, err := fs.Stat(absPath)
	if err != nil {
		return nil, utils.Wrapf(err, "cannot stat path[%s]", u.GetPath()) // check file / dir
	}
	res := f.fileInfoToResource(params.GetUrl(), query, currentInfo, absPath, false)
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     1,
		Resources: []*ypb.YakURLResource{res},
	}, nil
}

func (f fileSystemAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// available query:
	// trash=true # move to trash if supported
	u := params.GetUrl()
	fs := f.fs
	absPath, _, _, err := f.FormatPath(params)
	if err != nil {
		return nil, err
	}
	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	exists, err := fs.Exists(absPath)
	if !exists {
		return nil, utils.Errorf("file [%s] exists check error: %s", u.GetPath(), err)
	}
	if trash, ok := fs.(fi.TrashFileSystem); ok && query.Get("trash") == "true" {
		err = trash.Throw(absPath)
		if err != nil {
			return nil, err
		}
	} else {
		err = fs.Delete(absPath)
	}
	if err != nil {
		return nil, err
	}
	CheckUpdateFileMonitors(absPath)
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     0,
		Resources: []*ypb.YakURLResource{},
	}, nil
}

func (f fileSystemAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fileSystemAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (f *fileSystemAction) FormatPath(params *ypb.RequestYakURLParams) (string, string, string, error) {
	var (
		u                 = params.GetUrl()
		fs                = f.fs
		absPath           string
		dirname, filename string
	)
	path, err := codec.PathUnescape(u.GetPath())
	if err != nil {
		path = u.GetPath()
	}

	if fs.IsAbs(path) {
		dirname, filename = fs.PathSplit(path)
		absPath = path
	} else {
		wd, err := fs.Getwd()
		if err != nil {
			return "", "", "", utils.Wrap(err, "cannot get current working directory")
		}
		absPath = fs.Join(wd, path)
		dirname, filename = fs.PathSplit(absPath)
	}
	if remapped := f.remapSSAProgramLocalPath(absPath, path); remapped != "" {
		absPath = remapped
		dirname, filename = fs.PathSplit(absPath)
	}
	if params.GetUrl() != nil {
		params.Url.Path = absPath
	}
	return absPath, dirname, filename, nil
}

func (f *fileSystemAction) remapSSAProgramLocalPath(absPath, rawPath string) string {
	if f == nil || f.fs == nil {
		return ""
	}
	if _, ok := f.fs.(*filesys.LocalFs); !ok {
		return ""
	}
	if absPath != "" {
		if info, err := f.fs.Stat(absPath); err == nil && info != nil {
			return ""
		}
	}
	leaf := rawPath
	if absPath != "" {
		if _, name := f.fs.PathSplit(absPath); name != "" {
			leaf = name
		}
	}
	resolved := yakit.ResolveCodeSourceLocalPath(leaf)
	if resolved == "" {
		resolved = yakit.ResolveCodeSourceLocalPath(rawPath)
	}
	if resolved == "" || resolved == absPath {
		return ""
	}
	return resolved
}

func (f *fileSystemAction) FormatPathRaw(path string) (string, error) {
	var (
		fs      = f.fs
		absPath string
	)

	if fs.IsAbs(path) {
		absPath = path
	} else {
		wd, err := fs.Getwd()
		if err != nil {
			return "", utils.Wrap(err, "cannot get current working directory")
		}
		absPath = fs.Join(wd, path)
	}
	return absPath, nil
}
