package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

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

func Payload2Grpc(r *yakit.Payload) *ypb.Payload {
	content := getPayloadContent(r)
	p := &ypb.Payload{
		Id:           int64(r.ID),
		Group:        r.Group,
		ContentBytes: utils.UnsafeStringToBytes(content),
		Content:      utils.EscapeInvalidUTF8Byte(utils.UnsafeStringToBytes(content)),
		// Folder:       *r.Folder,
		// HitCount:     *r.HitCount,
		// IsFile:       *r.IsFile,
	}
	if r.Folder != nil {
		p.Folder = *r.Folder
	}
	if r.HitCount != nil {
		p.HitCount = *r.HitCount
	}
	if r.IsFile != nil {
		p.IsFile = *r.IsFile
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
		return nil, err
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
	if req.GetGroup() == "" {
		return nil, utils.Error("group name is empty")
	}
	filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetGroup())
	if err != nil {
		return nil, err
	}
	var size int64
	{
		if state, err := os.Stat(filename); err != nil {
			return nil, err
		} else {
			size += state.Size()
		}
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, utils.Errorf("failed to read file: %s", err)
	}

	reader := bufio.NewReader(f)
	outC := make(chan []byte)
	done := make(chan bool)
	go func() {
		defer f.Close()
		defer close(outC)
		for {
			select {
			case <-done:
				return
			default:
				lineRaw, err := utils.BufioReadLine(reader)
				if err != nil {
					return
				}
				raw := bytes.TrimSpace(lineRaw)
				outC <- raw
			}
		}
	}()
	var handlerSize int64 = 0

	defer close(done)

	data := make([]byte, 0, size)
	for line := range outC {
		handlerSize += int64(len(line))
		if s, err := strconv.Unquote(string(line)); err == nil {
			line = []byte(s)
		}
		line = append(line, "\r\n"...)
		data = append(data, line...)
		if size > FiveMB && handlerSize > OneKB {
			// If file is larger than 5MB, read only the first 50KB
			return &ypb.QueryPayloadFromFileResponse{
				Data:      data,
				IsBigFile: true,
			}, nil
		}
	}

	return &ypb.QueryPayloadFromFileResponse{
		Data:      data,
		IsBigFile: false,
	}, nil
}

func (s *Server) DeletePayloadByFolder(ctx context.Context, req *ypb.NameRequest) (*ypb.Empty, error) {
	if req.GetName() == "" {
		return nil, utils.Errorf("folder name is empty")
	}
	if err := yakit.DeletePayloadByFolder(s.GetProfileDatabase(), req.GetName()); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayloadByGroup(ctx context.Context, req *ypb.DeletePayloadByGroupRequest) (*ypb.Empty, error) {
	if req.GetGroup() == "" {
		return nil, utils.Errorf("group name is empty")
	}
	// if file, delete  file
	if group, err := yakit.GetPayloadByGroupFirst(s.GetProfileDatabase(), req.GetGroup()); err != nil {
		return nil, err
	} else {
		if group.IsFile != nil && *group.IsFile {
			// delete file
			if err := os.Remove(*group.Content); err != nil {
				return nil, err
			}
		}
	}
	// delete in database
	if err := yakit.DeletePayloadByGroup(s.GetProfileDatabase(), req.GetGroup()); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayload(ctx context.Context, req *ypb.DeletePayloadRequest) (*ypb.Empty, error) {
	if req.GetId() > 0 {
		if err := yakit.DeletePayloadByID(s.GetProfileDatabase(), req.GetId()); err != nil {
			return nil, utils.Wrap(err, "delete single line failed")
		}
	}

	if len(req.GetIds()) > 0 {
		if err := yakit.DeletePayloadByIDs(s.GetProfileDatabase(), req.GetIds()); err != nil {
			return nil, utils.Wrap(err, "delete multi line failed")
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) SavePayloadStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadStreamServer) (ret error) {
	if !req.IsFile && req.Content == "" {
		return utils.Error("content is empty")
	}
	if req.IsFile && len(req.FileName) == 0 {
		return utils.Error("file name is empty")
	}
	if req.Group == "" {
		return utils.Error("group is empty")
	}

	if req.IsNew {
		if ok, err := yakit.CheckExistGroup(s.GetProfileDatabase(), req.Group, req.Folder); err != nil {
			return utils.Wrapf(err, "check group[%s/%s]", req.Folder, req.Group)
		} else if ok {
			return utils.Errorf("group[%s/%s] exist", req.Folder, req.Group)
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
		db := s.GetProfileDatabase()
		db = db.Begin()
		err = yakit.SavePayloadByFilenameEx(f, func(data string, hitCount int64) error {
			size += int64(len(data))
			if total < size {
				total = size + 1
			}
			err := yakit.CreateOrUpdatePayload(db, data, req.GetGroup(), req.GetFolder(), hitCount, false)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			db.Rollback()
			return err
		}
		err = db.Commit().Error

		return err
	}

	defer func() {
		if total == 0 && ret == nil {
			ret = utils.Error("empty data no payload created")
		} else {
			feedback(1, "数据保存成功")
			yakit.SetGroupInEnd(s.GetProfileDatabase(), req.GetGroup())
		}
	}()
	if req.IsFile {
		for _, f := range req.FileName {
			err := handleFile(f)
			if err != nil {
				return utils.Wrapf(err, "handle file[%s] error", f)
			}
		}
	} else {
		// 旧接口
		total = int64(len(req.GetContent()))
		feedback(-1, "正在读取数据")
		if err := yakit.SavePayloadGroupByRawEx(req.GetContent(), func(data string) error {
			size += int64(len(data))
			if total < size {
				total = size + 1
			}
			return yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), data, req.GetGroup(), req.GetFolder(), 0, false)
		}); err != nil {
			log.Errorf("save payload group by content error: %s", err.Error())
		}
	}
	return nil
}

func (s *Server) SavePayloadToFileStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadToFileStreamServer) error {
	if !req.IsFile && req.Content == "" {
		return utils.Error("content is empty")
	}
	if req.IsFile && len(req.FileName) == 0 {
		return utils.Error("file name is empty")
	}
	if req.Group == "" {
		return utils.Error("group is empty")
	}

	if req.IsNew {
		if ok, err := yakit.CheckExistGroup(s.GetProfileDatabase(), req.Group, req.Folder); err != nil {
			return utils.Wrapf(err, "check group[%s/%s]", req.Folder, req.Group)
		} else if ok {
			return utils.Errorf("group[%s/%s] exist", req.Folder, req.Group)
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
		return yakit.SavePayloadByFilenameEx(f, saveDataByFilter)
	}

	if req.IsFile {
		feedback(0, "开始解析文件")
		for _, f := range req.FileName {
			if err := handleFile(f); err != nil {
				return utils.Wrapf(err, "handle file[%s] error", f)
			}
		}
	} else {
		total += int64(len(req.GetContent()))
		feedback(0, "开始解析数据")
		yakit.SavePayloadGroupByRawEx(req.GetContent(), saveDataByFilterNoHitCount)
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
	fileName := fmt.Sprintf("%s/%s_%s.txt", ProjectFolder, req.GetFolder(), req.GetGroup())
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
	folder := req.GetFolder()
	yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), fileName, req.GetGroup(), folder, 0, true)
	yakit.SetGroupInEnd(s.GetProfileDatabase(), req.GetGroup())
	if total == 0 {
		return utils.Error("empty data no payload created")
	}
	feedback(1, "导入完成")
	return nil
}

func (s *Server) RenamePayloadFolder(ctx context.Context, req *ypb.RenameRequest) (*ypb.Empty, error) {
	if req.GetName() == "" || req.GetNewName() == "" {
		return nil, utils.Error("old folder or folder can't be empty")
	}
	if err := yakit.RenamePayloadGroup(s.GetProfileDatabase(), getEmptyFolderName(req.GetName()), getEmptyFolderName(req.GetNewName())); err != nil {
		return nil, err
	}
	if err := yakit.RenamePayloadFolder(s.GetProfileDatabase(), req.GetName(), req.GetNewName()); err != nil {
		return nil, err
	} else {
		return &ypb.Empty{}, nil
	}
}

func (s *Server) RenamePayloadGroup(ctx context.Context, req *ypb.RenameRequest) (*ypb.Empty, error) {
	if req.GetName() == "" || req.GetNewName() == "" {
		return nil, utils.Error("group name and new name can't be empty")
	}

	if err := yakit.RenamePayloadGroup(s.GetProfileDatabase(), req.GetName(), req.GetNewName()); err != nil {
		return nil, err
	} else {
		return &ypb.Empty{}, nil
	}
}

func (s *Server) UpdatePayload(ctx context.Context, req *ypb.UpdatePayloadRequest) (*ypb.Empty, error) {
	id := req.GetId()
	data := req.GetData()
	var err error

	db := s.GetProfileDatabase()
	db = db.Begin()
	// just for old version
	if req.Group != "" || req.OldGroup != "" {
		err = yakit.RenamePayloadGroup(s.GetProfileDatabase(), req.OldGroup, req.Group)
		if err != nil {
			db.Rollback()
			return nil, err
		}
		err = db.Commit().Error
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}

	if id == 0 {
		return nil, utils.Error("id can't be empty")
	}
	if data == nil {
		return nil, utils.Error("data can't be empty")
	}
	if err := yakit.UpdatePayload(db, int(id), grpc2Payload(data)); err != nil {
		db.Rollback()
		return nil, err
	}
	err = db.Commit().Error
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func writeDataToFileEnd(filename, data string, flag int) error {
	file, err := os.OpenFile(filename, flag, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	state, err := file.Stat()
	if err != nil {
		return err
	}
	// 追加时，假如最后一个字符不是换行符，则添加换行符
	if flag&os.O_APPEND != 0 {
		buf := make([]byte, 1)
		file.ReadAt(buf, state.Size()-1)
		if buf[0] != '\n' {
			data = "\n" + data
		}
	}

	_, err = file.WriteString(data)
	if err != nil {
		return err
	}
	return nil
}

// rpc RemoveDuplicatePayloads(RemoveDuplicatePayloadsRequest) returns (stream SavePayloadProgress);
func (s *Server) RemoveDuplicatePayloads(req *ypb.NameRequest, stream ypb.Yak_RemoveDuplicatePayloadsServer) error {
	if req.GetName() == "" {
		return utils.Error("group can't be empty")
	}
	filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetName())
	if err != nil {
		return utils.Wrapf(err, "this group not a file payload group")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var handledSize, filtered, duplicate, total int64
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
	outC, err := utils.FileLineReader(filename)
	if err != nil {
		return err
	}

	data := make([]string, 0)
	filter := filter.NewFilter()

	for lineB := range outC {
		line := utils.UnsafeBytesToString(lineB)
		handledSize += int64(len(line))
		if total < handledSize {
			total = handledSize + 1
		}
		if !filter.Exist(line) {
			filtered++
			filter.Insert(line)
			data = append(data, line)
		} else {
			duplicate++
		}
	}

	feedback(0, "正在读取数据")
	feedback(0.99, fmt.Sprintf("检测到有%d项重复数据", duplicate))
	feedback(0.99, fmt.Sprintf("已去除重复数据, 剩余%d项数据", filtered))
	feedback(0.99, "正在保存到文件")
	defer feedback(1, "保存成功")
	if err := writeDataToFileEnd(filename, strings.Join(data, "\r\n"), os.O_WRONLY|os.O_TRUNC); err != nil {
		return err
	} else {
		return nil
	}
}

func (s *Server) UpdatePayloadToFile(ctx context.Context, req *ypb.UpdatePayloadToFileRequest) (*ypb.Empty, error) {
	if req.GetGroupName() == "" {
		return nil, utils.Error("group is empty")
	}
	if filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetGroupName()); err != nil {
		return nil, err
	} else {
		data := make([]string, 0)
		yakit.SavePayloadGroupByRawEx(req.GetContent(), func(s string) error {
			data = append(data, s)
			return nil
		})
		if err := writeDataToFileEnd(filename, strings.Join(data, "\r\n"), os.O_WRONLY|os.O_TRUNC); err != nil {
			return nil, err
		} else {
			return &ypb.Empty{}, nil
		}
	}
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

	groupFirstPayload, err := yakit.GetPayloadByGroupFirst(s.GetProfileDatabase(), req.GetGroup())
	if err != nil {
		return nil, err
	}

	var payloads []*yakit.Payload

	db := s.GetProfileDatabase().Model(&yakit.Payload{})
	ndb := bizhelper.ExactQueryInt64ArrayOr(db, "id", ids)
	if err := ndb.Find(&payloads).Error; err != nil {
		return nil, utils.Wrap(err, "error finding payloads")
	}
	db = db.Begin()
	if groupFirstPayload.IsFile != nil && *groupFirstPayload.IsFile {
		if groupFirstPayload.Content == nil || *groupFirstPayload.Content == "" {
			return nil, utils.Errorf("group [%s] is empty", group)
		}
		if !req.Copy {
			// if move to target
			// just delete original payload
			err = yakit.DeletePayloadByIDs(db, ids)
			if err != nil {
				db.Rollback()
				return nil, err
			}
		}
		for _, payload := range payloads {
			// write to target file payload group
			content := getPayloadContent(payload)
			if content == "" {
				continue
			}
			if err := writeDataToFileEnd(*groupFirstPayload.Content, content, os.O_RDWR|os.O_APPEND); err != nil {
				return nil, err
			}
		}
	} else {
		if req.Copy {
			err = yakit.CopyPayloads(db, payloads, group, folder)
		} else {
			err = yakit.MovePayloads(db, payloads, group, folder)
		}
		if err != nil {
			db.Rollback()
			return nil, err
		}
	}
	err = db.Commit().Error
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func getEmptyFolderName(folder string) string {
	return folder + "///empty"
}

func (s *Server) CreatePayloadFolder(ctx context.Context, req *ypb.NameRequest) (*ypb.Empty, error) {
	if req.Name == "" {
		return nil, utils.Errorf("name is Empty")
	}
	if err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), "", getEmptyFolderName(req.Name), req.Name, 0, false); err != nil {
		return nil, err
	} else {
		return &ypb.Empty{}, nil
	}
}

func (s *Server) UpdateAllPayloadGroup(ctx context.Context, req *ypb.UpdateAllPayloadGroupRequest) (*ypb.Empty, error) {
	nodes := req.Nodes
	folder := ""
	var index int64 = 0
	for _, node := range nodes {
		if node.Type == "Folder" {
			yakit.SetIndexToFolder(s.GetProfileDatabase(), node.Name, getEmptyFolderName(node.Name), index)
			folder = node.Name
			for _, child := range node.Nodes {
				yakit.UpdatePayloadGroup(s.GetProfileDatabase(), child.Name, folder, index)
				index++
			}
			folder = ""
		} else {
			yakit.UpdatePayloadGroup(s.GetProfileDatabase(), node.Name, folder, index)
		}
		index++
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetAllPayloadGroup(ctx context.Context, _ *ypb.Empty) (*ypb.GetAllPayloadGroupResponse, error) {
	type result struct {
		Group    string
		NumGroup int64
		Folder   *string
		IsFile   *bool
	}

	var res []result

	rows, err := s.GetProfileDatabase().Table("payloads").Select(`"group", COUNT("group") as num_group, folder, is_file`).Group(`"group"`).Order("group_index asc").Rows()
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var r result
		if err := rows.Scan(&r.Group, &r.NumGroup, &r.Folder, &r.IsFile); err != nil {
			return nil, err
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
	if req.GetGroup() == "" {
		return utils.Errorf("get all payload error: group is empty")
	}
	savePath := req.GetSavePath()
	if savePath == "" {
		return utils.Errorf("get all payload error: save path is empty")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	isCSV := strings.HasSuffix(savePath, ".csv")

	// 生成payload
	db := bizhelper.ExactQueryString(s.GetProfileDatabase(), "`group`", req.GetGroup())
	db = bizhelper.ExactQueryString(db, "`folder`", req.GetFolder())
	size, total := 0, 0
	n, hitCount := 0, int64(0)
	gen := yakit.YieldPayloads(db, context.Background())

	// 获取payload总长度
	if isCSV {
		contentSize, num, hitCountSize := 0, 0, 0
		db = s.GetProfileDatabase().Model(&yakit.Payload{}).Select("SUM(LENGTH(content)),COUNT(id),SUM(LENGTH(hit_count))").Where("`group` = ?", req.GetGroup()).Where("`folder` = ?", req.GetFolder())
		row := db.Row()
		row.Scan(&contentSize, &num, &hitCountSize)
		total = contentSize + num + hitCountSize
	} else {
		db = s.GetProfileDatabase().Model(&yakit.Payload{}).Select("SUM(LENGTH(content))").Where("`group` = ?", req.GetGroup()).Where("`folder` = ?", req.GetFolder())
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

	// 打开文件
	f, err := os.OpenFile(req.GetSavePath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return utils.Wrapf(err, "get all payload error: open file[%s] error", req.GetSavePath())
	}

	bufWriter := bufio.NewWriterSize(f, EightKB)
	defer func() {
		bufWriter.Flush()
		f.Close()
		feedback(1)
	}()
	bomHandled := false
	if isCSV {
		// 写入csv文件头
		bufWriter.WriteString("content,hit_count\n")
	}

	for p := range gen {
		content := getPayloadContent(p)
		if content == "" {
			continue
		}
		if !bomHandled {
			content = utils.RemoveBOMForString(content)
			bomHandled = true
		} else {
			bufWriter.WriteRune('\n')
		}
		if p.HitCount == nil {
			hitCount = 0
		} else {
			hitCount = *p.HitCount
		}
		if isCSV {
			n, _ = bufWriter.WriteString(fmt.Sprintf("%s,%d", content, hitCount))
		} else {
			n, _ = bufWriter.WriteString(content)
		}
		size += n
	}

	return nil
}

// 导出payload，从数据库中的文件导出到另外一个文件
func (s *Server) ExportAllPayloadFromFile(req *ypb.GetAllPayloadRequest, stream ypb.Yak_ExportAllPayloadFromFileServer) error {
	if req.GetGroup() == "" {
		return utils.Errorf("get all payload from file error: group is empty")
	}
	dst := req.GetSavePath()
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
	srcFile, err := os.Open(src)
	if err != nil {
		return utils.Wrapf(err, "get all payload from file error: open src file[%s] error", src)
	}
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return utils.Wrapf(err, "get all payload from file error: open dst file[%s] error", dst)
	}
	defer func() {
		srcFile.Close()
		dstFile.Close()
		feedback(1)
	}()

	bomHandled := false
	bufReader := bufio.NewReaderSize(srcFile, EightKB)

	isEOF := false
	for !isEOF {
		line, err := utils.BufioReadLine(bufReader)
		isEOF = errors.Is(err, io.EOF)
		if err != nil && !isEOF {
			return utils.Wrapf(err, "get all payload from file error: read file[%s] error", src)
		}
		content := utils.UnsafeBytesToString(line)
		unquoted, err := strconv.Unquote(content)
		if err == nil {
			content = unquoted
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if !bomHandled {
			content = utils.RemoveBOMForString(content)
			bomHandled = true
		} else {
			n, _ := dstFile.WriteString("\n")
			size += n
		}

		n, err := dstFile.WriteString(content)
		size += n
		if err != nil {
			return utils.Wrapf(err, "get all payload from file error: write file[%s] error", dst)
		}
	}

	return nil
}

func (s *Server) ConvertPayloadGroupToDatabase(req *ypb.NameRequest, stream ypb.Yak_ConvertPayloadGroupToDatabaseServer) error {
	if req.GetName() == "" {
		return utils.Errorf("group is empty")
	}

	group, err := yakit.GetPayloadByGroupFirst(s.GetProfileDatabase(), req.GetName())
	if err != nil {
		return err
	}
	if group.IsFile == nil && !*group.IsFile {
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
	if err := yakit.DeletePayloadByID(s.GetProfileDatabase(), int64(group.ID)); err != nil {
		return err
	}
	if group.Content == nil || *group.Content == "" {
		return utils.Error("this group filename is empty")
	}
	folder := ""
	if group.Folder != nil {
		folder = *group.Folder
	} else {
		utils.Error("this folder is nil, please try agin.")
	}
	var groupindex int64 = 0
	if group.GroupIndex != nil {
		groupindex = *group.GroupIndex
	} else {
		return utils.Error("this group index is empty, please try again.")
	}

	filename := *group.Content
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

	ch, err := utils.FileLineReader(filename)
	if err != nil {
		return err
	}

	for lineB := range ch {
		line := utils.UnsafeBytesToString(lineB)
		size += int64(len(line))
		yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), line, group.Group, folder, 0, false)
	}
	yakit.UpdatePayloadGroup(s.GetProfileDatabase(), group.Group, folder, groupindex)
	return nil
}

func (s *Server) MigratePayloads(req *ypb.Empty, stream ypb.Yak_MigratePayloadsServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	size, total := int64(0), int64(0)
	// 计算payload总数
	s.GetProfileDatabase().Model(&yakit.Payload{}).Count(&total)

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
	db = db.Begin()
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
		if err != nil { // 解码失败，可能是旧payload
			content = strconv.Quote(content)
			err = db.Model(&yakit.Payload{}).Where("`id` = ?", p.ID).Update("content", content).Error
			if err != nil {
				log.Errorf("update payload error: %v", err)
				continue
			}
		}
	}
	err := db.Commit().Error

	feedback(1)
	return err
}
