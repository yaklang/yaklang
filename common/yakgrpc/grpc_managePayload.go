package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
	OneMB   = 1 * 1024 * 1024 // 1 MB in bytes
	FiveMB  = 5 * 1024 * 1024 // 5 MB in bytes
)

type getAllPayloadResult struct {
	Group    string
	NumGroup int64
	Folder   *string
	IsFile   *bool
}

func grpc2Paging(pag *ypb.Paging) *yakit.Paging {
	ret := yakit.NewPaging()
	if pag != nil {
		ret.Order = pag.GetOrder()
		ret.OrderBy = pag.GetOrderBy()
		ret.Page = int(pag.GetPage())
		ret.Limit = int(pag.GetLimit())
	}
	return ret
}

func Payload2Grpc(payload *yakit.Payload) *ypb.Payload {
	content := getPayloadContent(payload)
	p := &ypb.Payload{
		Id:           int64(payload.ID),
		Group:        payload.Group,
		ContentBytes: utils.UnsafeStringToBytes(content),
		Content:      utils.EscapeInvalidUTF8Byte(utils.UnsafeStringToBytes(content)),
		// Folder:       *r.Folder,
		// HitCount:     *r.HitCount,
		// IsFile:       *r.IsFile,
	}
	if payload.Folder != nil {
		p.Folder = *payload.Folder
	}
	if payload.HitCount != nil {
		p.HitCount = *payload.HitCount
	}
	if payload.IsFile != nil {
		p.IsFile = *payload.IsFile
	}
	return p
}

func grpc2Payload(p *ypb.Payload) *yakit.Payload {
	content := strconv.Quote(p.Content)
	payload := &yakit.Payload{
		Group:    p.Group,
		Content:  &content,
		Folder:   &p.Folder,
		HitCount: &p.HitCount,
		IsFile:   &p.IsFile,
	}
	payload.Hash = payload.CalcHash()
	return payload
}

func getPayloadContent(p *yakit.Payload) string {
	if p.Content == nil {
		return ""
	}
	content := *p.Content
	unquoted, err := strconv.Unquote(content)
	if err == nil {
		content = unquoted
	}
	content = strings.TrimSpace(content)
	return content
}

func (s *Server) QueryPayload(ctx context.Context, req *ypb.QueryPayloadRequest) (*ypb.QueryPayloadResponse, error) {
	if req == nil {
		return nil, utils.Errorf("empty parameter")
	}
	p, d, err := yakit.QueryPayload(s.GetProfileDatabase(), req.GetFolder(), req.GetGroup(), req.GetKeyword(), grpc2Paging(req.GetPagination()))
	if err != nil {
		return nil, utils.Wrap(err, "query payload error")
	}

	var items []*ypb.Payload
	for _, r := range d {
		items = append(items, Payload2Grpc(r))
	}

	return &ypb.QueryPayloadResponse{
		Pagination: req.Pagination,
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

	lineCh, err := utils.FileLineReader(filename)
	if err != nil {
		return nil, utils.Errorf("failed to read file: %s", err)
	}

	var handlerSize int64 = 0

	buf := bytes.NewBuffer(make([]byte, 0, size))
	for line := range lineCh {
		lineStr := utils.UnsafeBytesToString(line)
		handlerSize += int64(len(line))
		if unquoted, err := strconv.Unquote(lineStr); err == nil {
			lineStr = unquoted
		}
		lineStr += "\n"
		buf.WriteString(lineStr)
		if size > FiveMB && handlerSize > OneKB {
			// If file is larger than 5MB, read only the first 50KB
			return &ypb.QueryPayloadFromFileResponse{
				Data:      buf.Bytes(),
				IsBigFile: true,
			}, nil
		}
	}

	return &ypb.QueryPayloadFromFileResponse{
		Data:      buf.Bytes(),
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
	// if file, delete  file
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

	if req.IsNew {
		if ok, err := yakit.CheckExistGroup(s.GetProfileDatabase(), group); err != nil {
			return utils.Wrapf(err, "check group[%s]", group)
		} else if ok {
			return utils.Errorf("group[%s] exist", group)
		}
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	size, total := int64(0), int64(0)
	start := time.Now()
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(size) / float64(total)
		}
		d := time.Since(start)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			CostDurationVerbose: d.String(),
			Message:             msg,
		})
	}
	ticker := time.NewTicker(500 * time.Millisecond)
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

	feedback(0, "start")
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
		feedback(-1, "正在读取文件: "+f)
		err = utils.GormTransaction(s.GetProfileDatabase(), func(tx *gorm.DB) error {
			return yakit.ReadPayloadFileLineWithCallBack(f, func(data string, hitCount int64) error {
				size += int64(len(data))
				if total < size {
					total = size + 1
				}
				err := yakit.CreateOrUpdatePayload(tx, data, group, folder, hitCount, false)
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
	if req.IsFile {
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
		if err := yakit.ReadQuotedLinesWithCallBack(content, func(data string) error {
			size += int64(len(data))
			if total < size {
				total = size + 1
			}
			return yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), data, group, folder, 0, false)
		}); err != nil {
			log.Errorf("save payload group by content error: %s", err.Error())
		}
	}
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

	if isNew {
		if ok, err := yakit.CheckExistGroup(s.GetProfileDatabase(), group); err != nil {
			return utils.Wrapf(err, "check group[%s]", group)
		} else if ok {
			return utils.Errorf("group[%s] exist", group)
		}
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var handledSize, filtered, duplicate, total int64
	feedback := func(progress float64, msg string) {
		if progress == -1 {
			progress = float64(handledSize) / float64(total)
		}
		stream.Send(&ypb.SavePayloadProgress{
			Progress: progress,
			Message:  msg,
		})
	}

	data := make([]struct {
		data     string
		hitCount int64
	}, 0)
	filter := filter.NewFilter()
	saveDataByFilter := func(s string, hitCount int64) error {
		handledSize += int64(len(s))
		if total < handledSize {
			total = handledSize + 1
		}
		if !filter.Exist(s) {
			filtered++
			filter.Insert(s)
			data = append(data,
				struct {
					data     string
					hitCount int64
				}{
					s, hitCount,
				})
		} else {
			duplicate++
		}
		return nil
	}
	saveDataByFilterNoHitCount := func(s string) error {
		return saveDataByFilter(s, 0)
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

		feedback(-1, "正在读取文件: "+f)
		return yakit.ReadPayloadFileLineWithCallBack(f, saveDataByFilter)
	}

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

	feedback(1, "step2")
	start := time.Now()
	feedback = func(progress float64, msg string) {
		if progress == 0 {
			progress = float64(handledSize) / float64(total)
		}
		d := time.Since(start)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			CostDurationVerbose: d.String(),
			Message:             msg,
		})
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
				feedback(-1, "")
			}
		}
	}()
	handledSize = 0
	total = int64(len(data))
	// save to file
	ProjectFolder := consts.GetDefaultYakitPayloadsDir()
	fileName := fmt.Sprintf("%s/%s_%s.txt", ProjectFolder, folder, group)
	fd, err := os.OpenFile(fileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	feedback(0, "正在写入文件")
	for i, d := range data {
		handledSize = int64(i)
		if i == int(total)-1 {
			fd.WriteString(d.data)
		} else {
			fd.WriteString(d.data + "\r\n")
		}
	}
	if err := fd.Close(); err != nil {
		return err
	}
	feedback(0.99, "写入文件完成")
	yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), fileName, group, folder, 0, true)
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
	data := req.GetData()
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
	if data == nil {
		return nil, utils.Error("data is empty")
	}
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		return yakit.UpdatePayload(tx, int(id), grpc2Payload(data))
	})
	return &ypb.Empty{}, err
}

func (s *Server) RemoveDuplicatePayloads(req *ypb.NameRequest, stream ypb.Yak_RemoveDuplicatePayloadsServer) error {
	group := req.GetName()
	if group == "" {
		return utils.Error("group is empty")
	}
	filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), group)
	if err != nil {
		return utils.Wrapf(err, "not a file payload group")
	}

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

	filter := filter.NewFilter()
	file, err := utils.NewFileLineWriter(filename, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return utils.Wrap(err, "open file for write payload error")
	}
	defer file.Close()

	feedback(0, "正在处理数据")
	for lineB := range lineCh {
		line := utils.UnsafeBytesToString(lineB)
		handledSize += int64(len(line))
		if total < handledSize {
			total = handledSize + 1
		}
		if filter.Exist(line) {
			duplicate++
			continue

		}
		filtered++
		filter.Insert(line)
		if _, err := file.WriteLineString(line); err != nil {
			return utils.Wrap(err, "write payload to file error")
		}
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

	err = yakit.ReadQuotedLinesWithCallBack(content, func(s string) error {
		if _, err := file.WriteLineString(s); err != nil {
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

	var payloads []*yakit.Payload

	db = db.Model(&yakit.Payload{})
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
			if !req.Copy {
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
				content := getPayloadContent(payload)
				if content == "" {
					continue
				}
				if _, err := file.WriteLineString(content); err != nil {
					return utils.Wrap(err, "write data to file error")
				}
			}
		} else {
			if req.Copy {
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

	rows, err := s.GetProfileDatabase().Model(&yakit.Payload{}).Select(`"group", COUNT("group") as num_group, folder, is_file`).Group(`"group"`).Order("group_index asc").Rows()
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
		db = s.GetProfileDatabase().Model(&yakit.Payload{}).Select("SUM(LENGTH(content)),COUNT(id),SUM(LENGTH(hit_count))").Where("`group` = ?", group).Where("`folder` = ?", folder)
		row := db.Row()
		row.Scan(&contentSize, &num, &hitCountSize)
		total = contentSize + num + hitCountSize
	} else {
		db = s.GetProfileDatabase().Model(&yakit.Payload{}).Select("SUM(LENGTH(content))").Where("`group` = ?", group).Where("`folder` = ?", folder)
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

	file, err := utils.NewFileLineWriter(req.GetSavePath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return utils.Wrapf(err, "get all payload error: open file[%s] error", req.GetSavePath())
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
		content := getPayloadContent(p)
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
	src, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetGroup())
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
		lineStr := utils.UnsafeBytesToString(line)
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
			CostDurationVerbose: d.String(),
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
			line := utils.UnsafeBytesToString(lineB)
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
	err := s.GetProfileDatabase().Model(&yakit.Payload{}).Count(&total).Error
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
	gen := yakit.YieldPayloads(s.GetProfileDatabase().Model(&yakit.Payload{}), ctx)
	db := s.GetProfileDatabase()
	utils.GormTransaction(db, func(tx *gorm.DB) error {
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
