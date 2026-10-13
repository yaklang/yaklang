package yakurl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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

// listDir lists a directory with the caller's query options: filesystems
// that understand them (filesys.QueryDirLister, e.g. ssadb://IrSourceFS
// with diffOnly=true) decide their own listing semantics, others (e.g.
// file://) ignore the query and list their storage directly.
func (f fileSystemAction) listDir(absPath string, query url.Values) ([]fs.DirEntry, error) {
	if ql, ok := f.fs.(filesys.QueryDirLister); ok {
		return ql.ReadDirQuery(absPath, query)
	}
	return f.fs.ReadDir(absPath)
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

// fsSearchMatch is one hit inside one line. Columns are 1-based and counted in
// UTF-16 code units of the line text published next to them, which is what a
// browser string index (and Monaco) uses, so the frontend can highlight the
// text as-is. EndColumn is exclusive.
type fsSearchMatch struct {
	StartColumn int `json:"start_column"`
	EndColumn   int `json:"end_column"`
}

// fsSearchLine is one matched line. A line matching several times stays a
// single entry and carries one fsSearchMatch per hit.
type fsSearchLine struct {
	Line    int             `json:"line"`
	Text    string          `json:"text"`
	Matches []fsSearchMatch `json:"matches"`
}

// fsSearchResult is the per-file search payload, published as JSON in the
// resource Extra under searchResultExtraKey.
type fsSearchResult struct {
	MatchCount int            `json:"match_count"`
	Results    []fsSearchLine `json:"results"`
}

// searchResultExtraKey is the Extra key of the JSON encoded fsSearchResult on
// every matched file. This replaced the old convention of one Extra entry per
// matched line (key = line number, value = whole line), which could not
// describe several hits on the same line nor where inside the line they were.
const searchResultExtraKey = "search_result"

// searchFileContent scans a file line by line and returns every matched line
// together with the location of each hit. Lines without a hit are skipped; the
// second return value counts individual hits, not lines.
func (f *fileSystemAction) searchFileContent(path string, matcher *regexp_utils.YakRegexpUtils) ([]fsSearchLine, int, error) {
	file, err := f.fs.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var (
		results    []fsSearchLine
		matchCount int
	)
	for lineNumber := 1; ; lineNumber++ {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			ranges, err := matcher.FindAllMatchIndex(line)
			if err != nil {
				return nil, 0, utils.Wrapf(err, "cannot match line %d", lineNumber)
			}
			if len(ranges) > 0 {
				result := newSearchLineResult(lineNumber, line, ranges)
				results = append(results, result)
				matchCount += len(result.Matches)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, 0, readErr
		}
	}
	return results, matchCount, nil
}

// newSearchLineResult turns the byte ranges found on a raw line into the form
// published to the frontend: the text to display plus one 1-based column range
// per hit inside that text.
func newSearchLineResult(lineNumber int, raw string, ranges [][]int) fsSearchLine {
	text, displayOffsets := escapeSearchLineForDisplay(raw)
	result := fsSearchLine{
		Line:    lineNumber,
		Text:    text,
		Matches: make([]fsSearchMatch, 0, len(ranges)),
	}
	for _, r := range ranges {
		if len(r) < 2 {
			continue
		}
		result.Matches = append(result.Matches, fsSearchMatch{
			StartColumn: utf16Column(text, displayOffsets[clampSearchOffset(r[0], len(raw))]),
			EndColumn:   utf16Column(text, displayOffsets[clampSearchOffset(r[1], len(raw))]),
		})
	}
	return result
}

func clampSearchOffset(offset, length int) int {
	if offset < 0 {
		return 0
	}
	if offset > length {
		return length
	}
	return offset
}

// utf16Column returns the 1-based UTF-16 column of a byte offset in text.
func utf16Column(text string, byteOffset int) int {
	if byteOffset <= 0 {
		return 1
	}
	if byteOffset > len(text) {
		byteOffset = len(text)
	}
	column := 1
	for _, r := range text[:byteOffset] {
		if r > 0xffff {
			column += 2
		} else {
			column++
		}
	}
	return column
}

// escapeSearchLineForDisplay mirrors utils.EscapeInvalidUTF8Byte — invalid UTF-8
// bytes and non-space control bytes become \xNN, because the text has to stay
// valid UTF-8 to travel inside a proto string — and additionally maps every raw
// byte offset to the matching offset in the escaped text. Escaping changes the
// length of a line, so hits found on the raw line can only be positioned in the
// text the frontend sees through that map.
func escapeSearchLineForDisplay(raw string) (string, []int) {
	offsets := make([]int, len(raw)+1)
	for i := range offsets {
		offsets[i] = -1
	}

	var builder strings.Builder
	builder.Grow(len(raw) + 8)
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[i:])
		offsets[i] = builder.Len()
		if r == utf8.RuneError && size <= 1 {
			builder.WriteString(fmt.Sprintf(`\x%02x`, raw[i]))
			i++
			continue
		}
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			builder.WriteString(fmt.Sprintf(`\x%02x`, byte(r)))
			i += size
			continue
		}
		builder.WriteString(raw[i : i+size])
		i += size
	}
	offsets[len(raw)] = builder.Len()

	// Bytes inside a multi-byte rune have no escape of their own; they share the
	// offset of the rune that starts them.
	last := 0
	for i := range offsets {
		if offsets[i] < 0 {
			offsets[i] = last
			continue
		}
		last = offsets[i]
	}
	return builder.String(), offsets
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
		lines, matchCount, err := f.searchFileContent(path, matcher)
		if err != nil {
			return utils.Wrapf(err, "cannot search file[%s]", path)
		}
		if len(lines) == 0 {
			return nil
		}

		payload, err := json.Marshal(&fsSearchResult{MatchCount: matchCount, Results: lines})
		if err != nil {
			return utils.Wrapf(err, "cannot encode search result of file[%s]", path)
		}
		resource := f.fileInfoToResource(originParam, query, info, path, true)
		resource.Extra = append(resource.Extra, &ypb.KVPair{
			Key:   searchResultExtraKey,
			Value: string(payload),
		})
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
		User:     originParam.User,
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
		// HaveChildrenNodes must agree with what listing the directory
		// returns under the same query: for an incremental program with
		// diffOnly, a directory whose entries all live in lower overlay
		// layers reports no children, matching the diff view the user sees.
		infos, err := f.listDir(currentPath, query)
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
	// regex=true # treat keyword as a regular expression (RE2, PCRE2 fallback)
	// ignoreCase=true # case-insensitive matching
	// search result Extra: "search_result" is JSON: matched lines with the column range of every hit
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
			infos, err := f.listDir(absPath, query)
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
