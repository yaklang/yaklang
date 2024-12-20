package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	OneKB   = 1 * 1024
	EightKB = 8 * 1024
	FiftyKB = 50 * 1024
	OneMB   = 1 * 1024 * 1024 // 1 MB in bytes
	FiveMB  = 5 * 1024 * 1024 // 5 MB in bytes
)

type getAllPayloadResult struct {
	Group    string
	NumGroup int64
	Folder   *string
	IsFile   *bool
}

func NewPagingFromGRPCModel(pag *ypb.Paging) *yakit.Paging {
	ret := yakit.NewPaging()
	if pag != nil {
		ret.Order = pag.GetOrder()
		ret.OrderBy = pag.GetOrderBy()
		ret.Page = int(pag.GetPage())
		ret.Limit = int(pag.GetLimit())
	}
	return ret
}

func (s *Server) QueryPayload(ctx context.Context, req *ypb.QueryPayloadRequest) (*ypb.QueryPayloadResponse, error) {
	if req == nil {
		return nil, utils.Errorf("empty parameter")
	}
	p, data, err := yakit.QueryPayload(s.GetProfileDatabase(), req.GetFolder(), req.GetGroup(), req.GetKeyword(), NewPagingFromGRPCModel(req.GetPagination()))
	if err != nil {
		return nil, utils.Wrap(err, "query payload error")
	}

	var items []*ypb.Payload
	for _, p := range data {
		items = append(items, p.ToGRPCModel())
	}

	return &ypb.QueryPayloadResponse{
		Pagination: req.GetPagination(),
		Total:      int64(p.TotalRecord),
		Data:       items,
	}, nil
}

func (s *Server) QueryPayloadFromFile(ctx context.Context, req *ypb.QueryPayloadFromFileRequest) (*ypb.QueryPayloadFromFileResponse, error) {
	group := req.GetGroup()
	if group == "" {
		return nil, utils.Error("group name is empty")
	}
	filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), group)
	if err != nil {
		return nil, utils.Wrap(err, "query payload from file error")
	}
	var size int64
	{
		if state, err := os.Stat(filename); err != nil {
			return nil, utils.Wrap(err, "query payload from file error")
		} else {
			size += state.Size()
		}
	}

	lineCh, err := utils.FileLineReaderWithContext(filename, ctx)
	if err != nil {
		return nil, utils.Errorf("failed to read file: %s", err)
	}

	var handlerSize int64 = 0

	buf := bytes.NewBuffer(make([]byte, 0, size))
	for line := range lineCh {
		lineStr := string(line)
		if unquoted, err := strconv.Unquote(lineStr); err == nil {
			lineStr = unquoted
		}
		lineStr += "\n"
		handlerSize += int64(len(lineStr) + 1)
		buf.WriteString(lineStr)
		if size > FiveMB && handlerSize > FiftyKB {
			// If file is larger than 5MB, read only the first 50KB
			return &ypb.QueryPayloadFromFileResponse{
				Data:      bytes.TrimRight(buf.Bytes(), "\n"),
				IsBigFile: true,
			}, nil
		}
	}

	return &ypb.QueryPayloadFromFileResponse{
		Data:      bytes.TrimRight(buf.Bytes(), "\n"),
		IsBigFile: false,
	}, nil
}

func (s *Server) DeletePayloadByFolder(ctx context.Context, req *ypb.NameRequest) (*ypb.Empty, error) {
	folder := req.GetName()
	if folder == "" {
		return nil, utils.Errorf("folder name is empty")
	}
	if err := yakit.DeletePayloadByFolder(s.GetProfileDatabase(), folder); err != nil {
		return nil, utils.Wrap(err, "delete payload by folder error")
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayloadByGroup(ctx context.Context, req *ypb.DeletePayloadByGroupRequest) (*ypb.Empty, error) {
	group := req.GetGroup()
	if group == "" {
		return nil, utils.Errorf("group name is empty")
	}
	// if file, delete file
	payload, err := yakit.GetPayloadFirst(s.GetProfileDatabase(), group)
	if err != nil {
		return nil, utils.Wrap(err, "delete payload by group error")
	}

	if payload.IsFile != nil && *payload.IsFile {
		// delete file
		if err := os.Remove(*payload.Content); err != nil {
			return nil, utils.Wrap(err, "delete payload by group error")
		}
	}

	// delete in database
	if err := yakit.DeletePayloadByGroup(s.GetProfileDatabase(), group); err != nil {
		return nil, utils.Wrap(err, "delete payload by group error")
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayload(ctx context.Context, req *ypb.DeletePayloadRequest) (*ypb.Empty, error) {
	id := req.GetId()
	ids := req.GetIds()

	if id > 0 {
		if err := yakit.DeletePayloadByID(s.GetProfileDatabase(), id); err != nil {
			return nil, utils.Wrap(err, "delete single line failed")
		}
	}

	if len(ids) > 0 {
		if err := yakit.DeletePayloadByIDs(s.GetProfileDatabase(), ids); err != nil {
			return nil, utils.Wrap(err, "delete multi line failed")
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) SavePayloadStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadStreamServer) (ret error) {
	content := req.GetContent()
	group := req.GetGroup()
	folder := req.GetFolder()
	isNew := req.GetIsNew()
	isFile := req.GetIsFile()
	filename := req.GetFileName()
	if !isFile && content == "" {
		return utils.Error("content is empty")
	}
	if isFile && len(filename) == 0 {
		return utils.Error("file name is empty")
	}
	if group == "" {
		return utils.Error("group is empty")
	}

	if payload, err := yakit.CheckExistGroup(s.GetProfileDatabase(), group); err != nil {
		if !isNew {
			return utils.Wrapf(err, "check group[%s]", group)
		}
	} else if payload != nil {
		if isNew {
			return utils.Errorf("group[%s] exist", group)
		} else if payload.Folder != nil && *payload.Folder != folder {
			return utils.Error("group folder not match, maybe need to upgrade yakit")
		}
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var sqlErr sqlite3.Error
	size, total := int64(0), int64(0)
	start := time.Now()
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(size) / float64(total)
		}
		d := time.Since(start)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			CostDurationVerbose: d.Round(time.Second).String(),
			Message:             msg,
		})
	}
	ticker := time.NewTicker(time.Second)
	go func() {
		defer func() {
			size = total
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feedback(-1, "")
			}
		}
	}()

	handleFile := func(f string) error {
		state, err := os.Stat(f)
		fileSize := state.Size()
		if err != nil {
			return err
		} else if fileSize == 0 {
			return errors.New("file is empty")
		}
		total += state.Size()

		defer feedback(-1, "文件 "+f+" 写入数据库成功")
		feedback(-1, "正在处理文件: "+f)
		err = utils.GormTransaction(s.GetProfileDatabase(), func(tx *gorm.DB) error {
			return yakit.ReadPayloadFileLineWithCallBack(ctx, f, func(data string, rawLen int64, hitCount int64) error {
				size += rawLen

				err := yakit.CreatePayload(tx, data, group, folder, hitCount, false)
				if errors.As(err, &sqlErr) && sqlErr.Code == sqlite3.ErrConstraint {
					err = nil // ignore duplicate error
				}
				return err
			})
		})

		return err
	}

	defer func() {
		if total == 0 && ret == nil {
			ret = utils.Error("empty data no payload created")
		} else {
			feedback(1, "数据保存成功")
			yakit.SetGroupInEnd(s.GetProfileDatabase(), group)
		}
	}()
	if isFile {
		for _, f := range filename {
			err := handleFile(f)
			if err != nil {
				return utils.Wrapf(err, "handle file[%s] error", f)
			}
		}
	} else {
		// 旧接口
		total = int64(len(content))
		feedback(-1, "正在读取数据")
		if err := yakit.ReadQuotedLinesWithCallBack(content, func(data string, rawLen int64) error {
			size += rawLen
			err := yakit.CreatePayload(s.GetProfileDatabase(), data, group, folder, 0, false)
			if errors.As(err, &sqlErr) && sqlErr.Code == sqlite3.ErrConstraint {
				err = nil // ignore duplicate error
			}
			return err
		}); err != nil {
			return utils.Wrapf(err, "save payload group by content error")
		}
	}
	return nil
}

func (s Server) SaveLargePayloadToFileStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SaveLargePayloadToFileStreamServer) error {
	content := req.GetContent()
	group := req.GetGroup()
	folder := req.GetFolder()
	isFile, isNew := req.GetIsFile(), req.GetIsNew()
	filename := req.GetFileName()
	if !isFile && content == "" {
		return utils.Error("content is empty")
	}
	if isFile && len(filename) == 0 {
		return utils.Error("file name is empty")
	}
	if group == "" {
		return utils.Error("group is empty")
	}

	if payload, err := yakit.CheckExistGroup(s.GetProfileDatabase(), group); err != nil {
		if !isNew {
			return utils.Wrapf(err, "check group[%s]", group)
		}
	} else if payload != nil {
		if isNew {
			return utils.Errorf("group[%s] exist", group)
		} else if payload.Folder != nil && *payload.Folder != folder {
			return utils.Error("group folder not match, maybe need to upgrade yakit")
		}
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	startTime := time.Now()
	payloadFolder := consts.GetDefaultYakitPayloadsDir()
	dstFileName := filepath.Join(payloadFolder, fmt.Sprintf("%s_%s.txt", folder, group))
	dstFD, err := os.OpenFile(dstFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	dstWriter := bufio.NewWriterSize(dstFD, oneMB)
	ch := make(chan string, 128)
	once := utils.NewAtomicBool()
	defer func() {
		dstWriter.Flush()
		dstFD.Close()
		if stream.Context().Err() == context.Canceled {
			os.Remove(dstFileName)
		}
	}()

	var handledSize, total int64
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(handledSize) / float64(total)
		}
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			CostDurationVerbose: time.Since(startTime).Round(time.Second).String(),
			Message:             msg,
		})
	}
	feedback(-1, "")

	ticker := time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				time.Sleep(time.Second)
				feedback(-1, "")
			}
		}
	}()

	var productWG sync.WaitGroup
	productWG.Add(len(filename))
	for _, f := range filename {
		f := f
		go func(f string) {
			defer func() {
				productWG.Done()
			}()
			yakit.ReadLargeFileLineWithCallBack(ctx, f,
				func(fi fs.FileInfo) {
					atomic.AddInt64(&total, fi.Size())
				},
				func(line string) error {
					atomic.AddInt64(&handledSize, int64(len(line)+1)) // +1 for '\n'
					line = strconv.Quote(line)
					ch <- line
					return nil
				})
		}(f)
	}

	// wait for all file read done
	go func() {
		productWG.Wait()
		close(ch)
	}()

	for s := range ch {
		if once.SetToIf(false, true) {
			dstWriter.WriteString(s)
		} else {
			dstWriter.WriteString("\n" + s)
		}
	}
	yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), dstFileName, group, folder, 0, true)
	yakit.SetGroupInEnd(s.GetProfileDatabase(), group)
	feedback(1, "导入完成")

	return nil
}

func (s *Server) SavePayloadToFileStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadToFileStreamServer) error {
	content := req.GetContent()
	group := req.GetGroup()
	folder := req.GetFolder()
	isFile, isNew := req.GetIsFile(), req.GetIsNew()
	filename := req.GetFileName()
	if !isFile && content == "" {
		return utils.Error("content is empty")
	}
	if isFile && len(filename) == 0 {
		return utils.Error("file name is empty")
	}
	if group == "" {
		return utils.Error("group is empty")
	}

	if payload, err := yakit.CheckExistGroup(s.GetProfileDatabase(), group); err != nil {
		if !isNew {
			return utils.Wrapf(err, "check group[%s]", group)
		}
	} else if payload != nil {
		if isNew {
			return utils.Errorf("group[%s] exist", group)
		} else if payload.Folder != nil && *payload.Folder != folder {
			return utils.Error("group folder not match, maybe need to upgrade yakit")
		}
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var (
		handledSize, filtered, duplicate, total int64
		startTime                               = time.Now()
	)
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(handledSize) / float64(total)
		}
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			CostDurationVerbose: time.Since(startTime).Round(time.Second).String(),
			Message:             msg,
		})
	}
	// dst
	dataFilter := filter.NewBigFilter()

	payloadFolder := consts.GetDefaultYakitPayloadsDir()
	dstFileName := filepath.Join(payloadFolder, fmt.Sprintf("%s_%s.txt", folder, group))
	dstFD, err := os.OpenFile(dstFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	dstWriter := bufio.NewWriterSize(dstFD, oneMB)
	defer func() {
		dstWriter.Flush()
		dstFD.Close()
		dataFilter.Close()
		if stream.Context().Err() == context.Canceled {
			os.Remove(dstFileName)
		}
	}()

	saveDataByFilter := func(s string, rawLen, hitCount int64) error {
		handledSize += rawLen
		newLine := true
		if handledSize >= total {
			newLine = false
		}

		if !dataFilter.Exist(s) {
			filtered++
			dataFilter.Insert(s)
			if _, err := dstWriter.WriteString(s); err != nil {
				return err
			}
			if newLine {
				if _, err := dstWriter.WriteString("\n"); err != nil {
					return err
				}
			}
		} else {
			duplicate++
		}
		return nil
	}
	saveDataByFilterNoHitCount := func(line string, rawLen int64) error {
		return saveDataByFilter(line, rawLen, 0)
	}

	handleFile := func(f string) error {
		state, err := os.Stat(f)
		fileSize := state.Size()
		if err != nil {
			return err
		} else if fileSize == 0 {
			return errors.New("file is empty")
		}
		total += state.Size()

		feedback(-1, "正在处理文件: "+f)
		return yakit.ReadPayloadFileLineWithCallBack(ctx, f, saveDataByFilter)
	}

	ticker := time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				time.Sleep(time.Second)
				feedback(-1, "")
			}
		}
	}()

	if isFile {
		feedback(0, "开始解析文件")
		for _, f := range filename {
			if err := handleFile(f); err != nil {
				return utils.Wrapf(err, "handle file[%s] error", f)
			}
		}
	} else {
		total += int64(len(content))
		feedback(0, "开始解析数据")
		yakit.ReadQuotedLinesWithCallBack(content, saveDataByFilterNoHitCount)
	}

	feedback(1, fmt.Sprintf("检测到有%d项重复数据", duplicate))
	feedback(1, fmt.Sprintf("已去除重复数据, 剩余%d项数据", filtered))

	yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), dstFileName, group, folder, 0, true)
	yakit.SetGroupInEnd(s.GetProfileDatabase(), group)
	if total == 0 {
		return utils.Error("empty data no payload created")
	}
	feedback(1, "导入完成")
	return nil
}

func (s *Server) RenamePayloadFolder(ctx context.Context, req *ypb.RenameRequest) (*ypb.Empty, error) {
	folder, newFolder := req.GetName(), req.GetNewName()
	if folder == "" {
		return nil, utils.Error("old folder is empty")
	}
	if newFolder == "" {
		return nil, utils.Error("new folder is empty")
	}
	db := s.GetProfileDatabase()
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := yakit.RenamePayloadGroup(tx, getEmptyFolderName(folder), getEmptyFolderName(newFolder)); err != nil {
			return utils.Wrap(err, "rename payload folder error")
		}
		if err := yakit.RenamePayloadFolder(tx, folder, newFolder); err != nil {
			return utils.Wrap(err, "rename payload folder error")
		}
		return nil
	})
	return &ypb.Empty{}, err
}

func (s *Server) RenamePayloadGroup(ctx context.Context, req *ypb.RenameRequest) (*ypb.Empty, error) {
	group, newGroup := req.GetName(), req.GetNewName()
	if group == "" {
		return nil, utils.Error("group name is empty")
	}
	if newGroup == "" {
		return nil, utils.Error("new group name is empty")
	}
	db := s.GetProfileDatabase()
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := yakit.RenamePayloadGroup(tx, req.GetName(), req.GetNewName()); err != nil {
			return utils.Wrap(err, "rename payload group error")
		}
		return nil
	})
	return &ypb.Empty{}, err
}

func (s *Server) UpdatePayload(ctx context.Context, req *ypb.UpdatePayloadRequest) (*ypb.Empty, error) {
	id := req.GetId()
	p := req.GetData()
	group, oldGroup := req.GetGroup(), req.GetOldGroup()

	db := s.GetProfileDatabase()

	// just for old version
	if group != "" || oldGroup != "" {
		err := utils.GormTransaction(db, func(tx *gorm.DB) error {
			err := yakit.RenamePayloadGroup(tx, oldGroup, group)
			return err
		})
		return &ypb.Empty{}, err
	}

	if id == 0 {
		return nil, utils.Error("id is empty")
	}
	if p == nil {
		return nil, utils.Error("data is empty")
	}
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		return yakit.UpdatePayload(tx, int(id), schema.NewPayloadFromGRPCModel(p))
	})
	return &ypb.Empty{}, err
}

func (s *Server) RemoveDuplicatePayloads(req *ypb.NameRequest, stream ypb.Yak_RemoveDuplicatePayloadsServer) error {
	group := req.GetName()
	if group == "" {
		return utils.Error("group is empty")
	}
	p, err := yakit.GetPayloadFirst(s.GetProfileDatabase(), group)
	// filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), group)
	if err != nil {
		return utils.Wrapf(err, "not a file payload group")
	}
	filename, folder := *p.Content, *p.Folder

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var (
		handledSize, filtered, duplicate int64
		total                            int64 = 1
	)
	if state, err := os.Stat(filename); err != nil {
		return err
	} else {
		total += state.Size()
	}
	total += 1
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(handledSize) / float64(total)
		}
		stream.Send(&ypb.SavePayloadProgress{
			Progress: progress,
			Message:  msg,
		})
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feedback(-1, "")
			}
		}
	}()
	lineCh, err := utils.FileLineReader(filename)
	if err != nil {
		return err
	}

	dataFilter := filter.NewBigFilter()
	defer dataFilter.Close()

	ProjectFolder := consts.GetDefaultYakitPayloadsDir()
	newFilename := filepath.Join(ProjectFolder, fmt.Sprintf("%s_%s_new.txt", folder, group))
	file, err := utils.NewFileLineWriter(newFilename, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return utils.Wrap(err, "open file for write payload error")
	}
	defer file.Close()

	feedback(0, "正在处理数据")
	for lineB := range lineCh {
		line := string(lineB)
		handledSize += int64(len(line))
		if total < handledSize {
			total = handledSize + 1
		}
		if dataFilter.Exist(line) {
			duplicate++
			continue

		}
		filtered++
		dataFilter.Insert(line)
		if _, err := file.WriteLineString(line); err != nil {
			return utils.Wrap(err, "write payload to file error")
		}
	}
	feedback(0.99, "正在覆写文件")
	if err := os.RemoveAll(filename); err != nil {
		return utils.Wrap(err, "remove old file error")
	}
	file.Close()
	if err := os.Rename(newFilename, filename); err != nil {
		return utils.Wrap(err, "rename new file error")
	}

	feedback(0.99, fmt.Sprintf("总共%d项数据，重复%d项数据，实际写入%d项数据", filtered+duplicate, duplicate, filtered))
	feedback(1, "保存成功")

	return nil
}

func (s *Server) UpdatePayloadToFile(ctx context.Context, req *ypb.UpdatePayloadToFileRequest) (*ypb.Empty, error) {
	group := req.GetGroupName()
	content := req.GetContent()
	if group == "" {
		return nil, utils.Error("group is empty")
	}

	filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), group)
	if err != nil {
		return nil, utils.Wrap(err, "get payload filename error")
	}

	file, err := utils.NewFileLineWriter(filename, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, utils.Wrap(err, "open file for write payload error")
	}
	defer file.Close()

	err = yakit.ReadQuotedLinesWithCallBack(content, func(line string, rawLen int64) error {
		if _, err := file.WriteLineString(line); err != nil {
			return utils.Wrap(err, "write payload to file error")
		}
		return nil
	})
	return &ypb.Empty{}, err
}

func (s *Server) BackUpOrCopyPayloads(ctx context.Context, req *ypb.BackUpOrCopyPayloadsRequest) (*ypb.Empty, error) {
	ids := req.GetIds()
	group := req.GetGroup()
	folder := req.GetFolder()
	isCopy := req.GetCopy()

	if len(ids) == 0 {
		return nil, utils.Error("ids is empty")
	}
	if group == "" {
		return nil, utils.Error("group is empty")
	}
	db := s.GetProfileDatabase()

	groupFirstPayload, err := yakit.GetPayloadFirst(db, group)
	if err != nil {
		return nil, err
	}

	var payloads []*schema.Payload

	db = db.Model(&schema.Payload{})
	if err := bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Find(&payloads).Error; err != nil {
		return nil, utils.Wrap(err, "error finding payloads")
	}

	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		var err error
		if groupFirstPayload.IsFile != nil && *groupFirstPayload.IsFile {
			if groupFirstPayload.Content == nil || *groupFirstPayload.Content == "" {
				return utils.Errorf("group [%s] is empty", group)
			}
			filename := *groupFirstPayload.Content
			if !isCopy {
				// if move to target
				// just delete original payload
				err = yakit.DeletePayloadByIDs(tx, ids)
				if err != nil {
					return utils.Wrap(err, "delete payload error")
				}
			}
			file, err := utils.NewFileLineWriter(filename, os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return utils.Wrap(err, "open file for write payload error")
			}
			defer file.Close()
			for _, payload := range payloads {
				// write to target file payload group
				content := payload.GetContent()
				if content == "" {
					continue
				}
				if _, err := file.WriteLineString(content); err != nil {
					return utils.Wrap(err, "write data to file error")
				}
			}
		} else {
			if isCopy {
				err = yakit.CopyPayloads(tx, payloads, group, folder)
			} else {
				err = yakit.MovePayloads(tx, payloads, group, folder)
			}
		}
		return err
	})

	return &ypb.Empty{}, err
}

func getEmptyFolderName(folder string) string {
	return folder + "///empty"
}

func (s *Server) CreatePayloadFolder(ctx context.Context, req *ypb.NameRequest) (*ypb.Empty, error) {
	folder := req.GetName()
	if folder == "" {
		return nil, utils.Errorf("name is Empty")
	}
	if err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), "", getEmptyFolderName(folder), folder, 0, false); err != nil {
		return nil, utils.Wrap(err, "create payload folder error")
	} else {
		return &ypb.Empty{}, nil
	}
}

func (s *Server) UpdateAllPayloadGroup(ctx context.Context, req *ypb.UpdateAllPayloadGroupRequest) (*ypb.Empty, error) {
	var (
		index int64 = 0
		err   error
	)
	nodes := req.Nodes
	folder := ""
	db := s.GetProfileDatabase()
	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, node := range nodes {
			if node.Type == "Folder" {
				folder = node.Name
				yakit.SetIndexToFolder(tx, folder, getEmptyFolderName(folder), index)
				for _, child := range node.Nodes {
					err = yakit.UpdatePayloadGroup(tx, child.Name, folder, index)
					if err != nil {
						return utils.Wrap(err, "update payload group error")
					}
					index++
				}
				folder = ""
			} else {
				err = yakit.UpdatePayloadGroup(tx, node.Name, folder, index)
				if err != nil {
					return utils.Wrap(err, "update payload group error")
				}
			}
			index++
		}
		return nil
	})
	return &ypb.Empty{}, err
}

func (s *Server) GetAllPayloadGroup(ctx context.Context, _ *ypb.Empty) (*ypb.GetAllPayloadGroupResponse, error) {
	var res []getAllPayloadResult

	rows, err := s.GetProfileDatabase().Model(&schema.Payload{}).Select(`"group", COUNT("group") as num_group, folder, is_file`).Group(`"group"`).Order("group_index asc").Rows()
	if err != nil {
		return nil, utils.Wrap(err, "get all payload group error")
	}

	for rows.Next() {
		var r getAllPayloadResult
		if err := rows.Scan(&r.Group, &r.NumGroup, &r.Folder, &r.IsFile); err != nil {
			return nil, utils.Wrap(err, "get all payload group error")
		}
		res = append(res, r)
	}

	groups := make([]string, 0, len(res))
	nodes := make([]*ypb.PayloadGroupNode, 0)
	folders := make(map[string]*ypb.PayloadGroupNode)
	add2Folder := func(folder string, node *ypb.PayloadGroupNode) (ret *ypb.PayloadGroupNode) {
		// skip group="" payload, this is empty folder
		folderNode, ok := folders[folder]
		if !ok {
			folderNode = &ypb.PayloadGroupNode{
				Type:   "Folder",
				Name:   folder,
				Number: 0,
				Nodes:  make([]*ypb.PayloadGroupNode, 0),
			}
			folders[folder] = folderNode
			ret = folderNode
		}
		if node.Name != getEmptyFolderName(folder) {
			folderNode.Nodes = append(folderNode.Nodes, node)
			folderNode.Number += node.Number
		}
		return
	}
	for _, r := range res {
		if r.Folder != nil && r.Group != getEmptyFolderName(*r.Folder) {
			groups = append(groups, r.Group)
		}
		typ := "DataBase"
		if r.IsFile != nil && *r.IsFile {
			typ = "File"
		}

		node := &ypb.PayloadGroupNode{
			Type:   typ,
			Name:   r.Group,
			Number: r.NumGroup,
			Nodes:  nil,
		}
		if r.Folder != nil && *r.Folder != "" {
			if n := add2Folder(*r.Folder, node); n != nil {
				nodes = append(nodes, n)
			}
		} else {
			nodes = append(nodes, node)
		}
	}

	return &ypb.GetAllPayloadGroupResponse{
		Groups: groups,
		Nodes:  nodes,
	}, nil
}

// ! 已弃用
// 导出payload到文件
func (s *Server) GetAllPayload(ctx context.Context, req *ypb.GetAllPayloadRequest) (*ypb.GetAllPayloadResponse, error) {
	if req.GetGroup() == "" {
		return nil, utils.Errorf("group is empty")
	}
	db := bizhelper.ExactQueryString(s.GetProfileDatabase(), "`group`", req.GetGroup())
	var payloads []*ypb.Payload
	gen := yakit.YieldPayloads(db, context.Background())

	for p := range gen {
		payloads = append(payloads, &ypb.Payload{
			Content: p.GetContent(),
		})
	}

	return &ypb.GetAllPayloadResponse{
		Data: payloads,
	}, nil
}

// 导出payload到文件
func (s *Server) ExportAllPayload(req *ypb.GetAllPayloadRequest, stream ypb.Yak_ExportAllPayloadServer) error {
	group := req.GetGroup()
	folder := req.GetFolder()
	savePath := req.GetSavePath()
	if group == "" {
		return utils.Errorf("get all payload error: group is empty")
	}
	if savePath == "" {
		return utils.Errorf("get all payload error: save path is empty")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	isCSV := strings.HasSuffix(savePath, ".csv")

	// 生成payload
	db := s.GetProfileDatabase().Where("`group` = ?", group).Where("`folder` = ?", folder)
	size, total := 0, 0
	n, hitCount := 0, int64(0)
	gen := yakit.YieldPayloads(db, context.Background())

	// 获取payload总长度
	if isCSV {
		contentSize, num, hitCountSize := 0, 0, 0
		db = s.GetProfileDatabase().Model(&schema.Payload{}).Select("SUM(LENGTH(content)),COUNT(id),SUM(LENGTH(hit_count))").Where("`group` = ?", group).Where("`folder` = ?", folder)
		row := db.Row()
		row.Scan(&contentSize, &num, &hitCountSize)
		total = contentSize + num + hitCountSize
	} else {
		db = s.GetProfileDatabase().Model(&schema.Payload{}).Select("SUM(LENGTH(content))").Where("`group` = ?", group).Where("`folder` = ?", folder)
		row := db.Row()
		row.Scan(&total)
	}
	if total == 0 {
		return utils.Errorf("get all payload error: group not exist payload(s)")
	}
	feedback := func(progress float64) {
		if progress == -1 {
			progress = float64(size) / float64(total)
		}
		stream.Send(&ypb.GetAllPayloadResponse{
			Progress: progress,
		})
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feedback(-1)
			}
		}
	}()

	file, err := utils.NewFileLineWriter(savePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return utils.Wrapf(err, "get all payload error: open file[%s] error", savePath)
	}
	defer func() {
		file.Close()
		feedback(1)
	}()
	bomHandled := false
	if isCSV {
		// 写入csv文件头
		file.WriteLineString("content,hit_count")
	}

	for p := range gen {
		content := p.GetContent()
		if content == "" {
			continue
		}
		if !bomHandled {
			content = utils.RemoveBOMForString(content)
			bomHandled = true
		}
		if p.HitCount == nil {
			hitCount = 0
		} else {
			hitCount = *p.HitCount
		}
		if isCSV {
			n, _ = file.WriteLineString(fmt.Sprintf("%s,%d", content, hitCount))
		} else {
			n, _ = file.WriteLineString(content)
		}
		size += n
	}

	return nil
}

// 导出payload，从数据库中的文件导出到另外一个文件
func (s *Server) ExportAllPayloadFromFile(req *ypb.GetAllPayloadRequest, stream ypb.Yak_ExportAllPayloadFromFileServer) error {
	group := req.GetGroup()
	dst := req.GetSavePath()
	if group == "" {
		return utils.Errorf("get all payload from file error: group is empty")
	}
	if dst == "" {
		return utils.Errorf("get all payload from file error: save path is empty")
	}
	src, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), group)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// 获取payload总长度
	size, total := 0, 0
	state, err := os.Stat(src)
	if err != nil {
		return utils.Wrap(err, "get all payload from file error: get file state error")
	}
	total = int(state.Size())

	feedback := func(progress float64) {
		if progress == -1 {
			progress = float64(size) / float64(total)
		}
		stream.Send(&ypb.GetAllPayloadResponse{
			Progress: progress,
		})
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feedback(-1)
			}
		}
	}()

	// 打开源文件和目标文件

	lineC, err := utils.FileLineReader(src)
	if err != nil {
		return utils.Wrapf(err, "get all payload from file error: open src file[%s] error", src)
	}
	file, err := utils.NewFileLineWriter(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return utils.Wrapf(err, "get all payload from file error: open dst file[%s] error", dst)
	}
	defer func() {
		file.Close()
		feedback(1)
	}()

	bomHandled := false
	for line := range lineC {
		if !bomHandled {
			line = utils.RemoveBOM(line)
			bomHandled = true
		}
		lineStr := string(line)
		unquoted, err := strconv.Unquote(lineStr)
		if err == nil {
			lineStr = unquoted
		}

		n, _ := file.WriteLineString(lineStr)
		size += n
	}

	return nil
}

func (s *Server) ConvertPayloadGroupToDatabase(req *ypb.NameRequest, stream ypb.Yak_ConvertPayloadGroupToDatabaseServer) error {
	group := req.GetName()
	if group == "" {
		return utils.Errorf("group is empty")
	}

	payload, err := yakit.GetPayloadFirst(s.GetProfileDatabase(), group)
	if err != nil {
		return err
	}
	if payload.IsFile == nil && !*payload.IsFile {
		return utils.Errorf("group is not file")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var size, total int64
	_ = size
	start := time.Now()
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(size) / float64(total)
		}
		d := time.Since(start)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			CostDurationVerbose: d.Round(time.Second).String(),
			Message:             msg,
		})
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feedback(-1, "")
			}
		}
	}()

	feedback(0, "start")
	if err := yakit.DeletePayloadByID(s.GetProfileDatabase(), int64(payload.ID)); err != nil {
		return err
	}
	if payload.Content == nil || *payload.Content == "" {
		return utils.Error("this group filename is empty")
	}
	folder := ""
	if payload.Folder != nil {
		folder = *payload.Folder
	} else {
		utils.Error("this folder is nil, please try agin.")
	}
	var groupIndex int64 = 0
	if payload.GroupIndex != nil {
		groupIndex = *payload.GroupIndex
	} else {
		return utils.Error("this group index is empty, please try again.")
	}

	filename := *payload.Content
	if state, err := os.Stat(filename); err != nil {
		return err
	} else {
		total += state.Size()
	}
	feedback(-1, "正在读取文件: "+filename)
	defer func() {
		feedback(1, "转换完成, 该Payload字典已经转换为数据库存储。")
		os.Remove(filename)
	}()

	lineCh, err := utils.FileLineReader(filename)
	if err != nil {
		return err
	}
	db := s.GetProfileDatabase()
	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		for lineB := range lineCh {
			line := string(lineB)
			size += int64(len(line))
			err = yakit.CreateOrUpdatePayload(tx, line, payload.Group, folder, 0, false)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		return yakit.UpdatePayloadGroup(tx, payload.Group, folder, groupIndex)
	})
	return err
}

func (s *Server) MigratePayloads(req *ypb.Empty, stream ypb.Yak_MigratePayloadsServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	size, total := int64(0), int64(0)
	// 计算payload总数
	err := s.GetProfileDatabase().Model(&schema.Payload{}).Count(&total).Error
	if err != nil {
		return utils.Wrap(err, "migrate payload error: get payload count error")
	}

	feedback := func(progress float64) {
		if progress == -1 {
			progress = float64(size) / float64(total)
		}
		stream.Send(&ypb.SavePayloadProgress{
			Progress: progress,
			Message:  "正在迁移数据库...",
		})
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feedback(-1)
			}
		}
	}()

	feedback(0)
	db := s.GetProfileDatabase().Model(&schema.Payload{})
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		gen := yakit.YieldPayloads(tx, ctx)
		for p := range gen {
			size++
			if p.Content == nil || (p.IsFile != nil && *p.IsFile) {
				continue
			}
			content := *p.Content
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			// 开始迁移
			_, err := strconv.Unquote(content)
			if err == nil {
				continue
			}
			// 解码失败，可能是旧payload
			content = strconv.Quote(content)
			if err := yakit.UpdatePayloadColumns(tx, int(p.ID), map[string]any{"content": content}); err != nil {
				log.Errorf("update payload error: %v", err)
				continue
			}
		}
		return nil
	})

	feedback(1)
	return err
}
