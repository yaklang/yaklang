package yakgrpc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	raw, err := strconv.Unquote(*r.Content)
	if err != nil {
		raw = *r.Content
	}
	return &ypb.Payload{
		Id:           int64(r.ID),
		Group:        r.Group,
		ContentBytes: []byte(raw),
		Content:      utils.EscapeInvalidUTF8Byte([]byte(raw)),
		Folder:       *r.Folder,
		HitCount:     *r.HitCount,
		IsFile:       *r.IsFile,
	}
}
func grpc2Payload(p *ypb.Payload) *yakit.Payload {
	payload := &yakit.Payload{
		Group:    p.Group,
		Content:  &p.Content,
		Folder:   &p.Folder,
		HitCount: &p.HitCount,
		IsFile:   &p.IsFile,
	}
	payload.Hash = payload.CalcHash()
	return payload
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

const (
	FiveMB  = 5 * 1024 * 1024 // 5 MB in bytes
	FiftyKB = 50 * 1024       // 50 KB in bytes
)

func (s *Server) QueryPayloadFromFile(ctx context.Context, req *ypb.QueryPayloadFromFileRequest) (*ypb.QueryPayloadFromFileResponse, error) {
	if req.GetGroup() == "" {
		return nil, utils.Error("group name is empty")
	}
	filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetGroup())
	if err != nil {

		return nil, err
	}
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	fstat, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	size := fstat.Size()
	if size > FiveMB {
		// If file is larger than 5MB, read only the first 50KB
		buf := make([]byte, FiftyKB)
		_, err = fd.Read(buf)
		if err != nil {
			return nil, err
		}

		return &ypb.QueryPayloadFromFileResponse{
			Data:      buf,
			IsBigFile: true,
		}, nil
	} else {
		// If file is smaller than 5MB, read the whole file
		contentBytes, err := ioutil.ReadAll(fd)
		if err != nil {
			return nil, err
		}

		return &ypb.QueryPayloadFromFileResponse{
			Data:      contentBytes,
			IsBigFile: false,
		}, nil
	}
}

func (s *Server) DeletePayloadByFolder(ctx context.Context, req *ypb.NameRequest) (*ypb.Empty, error) {
	if req.GetName() == "" {
		return nil, utils.Errorf("folder name is empty ")
	}
	if err := yakit.DeletePayloadByFolder(s.GetProfileDatabase(), req.GetName()); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayloadByGroup(ctx context.Context, req *ypb.NameRequest) (*ypb.Empty, error) {
	if req.GetName() == "" {
		return nil, utils.Errorf("group name is empty ")
	}
	if err := yakit.DeletePayloadByGroup(s.GetProfileDatabase(), req.GetName()); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayload(ctx context.Context, req *ypb.DeletePayloadByIdRequest) (*ypb.Empty, error) {
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

const (
	OneMB = 1 * 1024 * 1024 // 5 MB in bytes
)

func (s *Server) SavePayloadStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadStreamServer) error {
	if (!req.IsFile && req.Content == "") || (req.IsFile && len(req.FileName) == 0) || (req.Group == "") {
		return utils.Error("content or file name or Group is empty ")
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
		speed := float64((size)/OneMB) / (d.Seconds())
		rest := float64((total-size)/OneMB) / (speed)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			Speed:               fmt.Sprintf("%f", speed),
			CostDurationVerbose: d.String(),
			RestDurationVerbose: fmt.Sprintf("%f", rest),
			Message:             msg,
		})
	}
	// _ = feedback
	go func() {
		defer func() {
			size = total
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(500 * time.Millisecond)
				feedback(float64(size)/float64(total), "")
			}
		}
	}()

	feedback(0, "start")
	handleFile := func(f string) error {
		if state, err := os.Stat(f); err != nil {
			return err
		} else {
			total += state.Size()
		}
		defer feedback(-1, "文件 "+f+" 写入数据库成功")
		feedback(-1, "正在读取文件: "+f)
		return yakit.SavePayloadByFilenameEx(s.GetProfileDatabase(), req.GetGroup(), f, func(data string, hitCount int64) error {
			size += int64(len(data))
			return yakit.CreateAndUpdatePayload(s.GetProfileDatabase(), data, req.GetGroup(), req.GetFolder(), hitCount)
		})
	}

	defer feedback(1, "数据保存成功")

	if req.IsFile {
		for _, f := range req.FileName {
			err := handleFile(f)
			if err != nil {
				log.Errorf("handle file %s error: %s", f, err.Error())
				continue
			}
		}
	} else {
		total = int64(len(req.GetContent()))
		feedback(-1, "正在读取数据 ")
		if err := yakit.SavePayloadGroupByRawEx(s.GetProfileDatabase(), req.GetGroup(), req.GetContent(), func(data string) error {
			size += int64(len(data))
			return yakit.CreateAndUpdatePayload(s.GetProfileDatabase(), data, req.GetGroup(), req.GetFolder(), 0)
		}); err != nil {
			log.Errorf("save payload group by content error: %s", err.Error())
		}
	}
	return nil
}

func (s *Server) SavePayloadToFileStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadToFileStreamServer) error {
	if (!req.IsFile && req.Content == "") || (req.IsFile && len(req.FileName) == 0) || (req.Group == "") {
		return utils.Error("content and file name all is empty ")
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	var handledSize, filtered, total int64
	feedback := func(progress float64, msg string) {
		if progress == 0 {
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
		handledSize++
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
		}
		return nil
	}
	saveDataByFilterNoHitCount := func(s string) error {
		return saveDataByFilter(s, 0)
	}

	handleFile := func(f string) error {
		if state, err := os.Stat(f); err != nil {
			return err
		} else {
			total += state.Size()
		}
		feedback(0, "正在读取文件: "+f)
		return yakit.SavePayloadByFilenameEx(s.GetProfileDatabase(), req.GetGroup(), f, saveDataByFilter)
	}

	if req.IsFile {
		for _, file := range req.FileName {
			if err := handleFile(file); err != nil {
				log.Errorf("open file %s error: %s", file, err.Error())
			}
		}
	} else {
		total += int64(len(req.GetContent()))
		feedback(0, "正在读取数据")
		yakit.SavePayloadGroupByRawEx(s.GetProfileDatabase(), req.GetGroup(), req.GetContent(), saveDataByFilterNoHitCount)
	}

	feedback(0, fmt.Sprintf("检测到有%d项重复数据", total-filtered))
	feedback(0, fmt.Sprintf("已去除重复数据, 剩余%d项数据", filtered))

	feedback(1, "step2")
	start := time.Now()
	feedback = func(progress float64, msg string) {
		if progress == 0 {
			progress = float64(handledSize) / float64(total)
		}
		d := time.Since(start)
		speed := float64((handledSize)/OneMB) / (d.Seconds())
		rest := float64((total-handledSize)/OneMB) / (speed)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            progress,
			Speed:               fmt.Sprintf("%f", speed),
			CostDurationVerbose: d.String(),
			RestDurationVerbose: fmt.Sprintf("%f", rest),
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
				feedback(0, "")
			}
		}
	}()
	handledSize = 0
	total = int64(len(data))
	// save to file
	ProjectFolder := ""
	fileName := fmt.Sprintf("%s/%s_%s.txt", ProjectFolder, req.GetFolder(), req.GetGroup())
	fd, err := os.OpenFile(fileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	feedback(0, "正在写入文件")
	for i, d := range data {
		handledSize = int64(i)
		feedback(0, "")
		fd.WriteString(d.data + "\r\n")
	}
	if err := fd.Close(); err != nil {
		return err
	}
	feedback(0, "写入文件完成")
	folder := req.GetFolder()
	f := true
	payload := yakit.NewPayload(req.GetGroup(), fileName)
	payload.Folder = &folder
	payload.IsFile = &f
	yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload)
	feedback(1, "导入完成")
	return nil
}

func (s *Server) RenamePayloadFolder(ctx context.Context, req *ypb.RenameRequest) (*ypb.Empty, error) {
	if req.GetName() == "" || req.GetNewName() == "" {
		return nil, utils.Error("old folder or folder can't be empty")
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
	if req.GetId() == 0 || req.GetData() == nil {
		return nil, utils.Error("id or data can't be empty")
	}
	if err := yakit.UpdatePayload(s.GetProfileDatabase(), int(req.GetId()), grpc2Payload(req.GetData())); err != nil {
		return nil, err
	} else {
		return &ypb.Empty{}, nil
	}
}

func appendDataToFileEnd(filename, data string) error {
	// Open the file in append mode.
	// If the file doesn't exist, create it.
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write the data to the file.
	_, err = file.WriteString(data)
	if err != nil {
		return err
	}
	return nil
}
func (s *Server) UpdatePayloadToFile(ctx context.Context, req *ypb.UpdatePayloadToFileRequest) (*ypb.Empty, error) {
	if req.GetGroupName() == "" || req.GetContent() == "" {
		return nil, utils.Error("id or data can't be empty")
	}
	if filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetGroupName()); err != nil {
		return nil, err
	} else {
		if err := appendDataToFileEnd(filename, req.GetContent()); err != nil {
			return nil, err
		} else {
			return &ypb.Empty{}, nil
		}
	}
}

func (s *Server) BackUpOrCopyPayloads(ctx context.Context, req *ypb.BackUpOrCopyPayloadsRequest) (*ypb.Empty, error) {
	if len(req.GetIds()) == 0 || req.GetGroup() == "" {
		return nil, utils.Error("id or group name can't be empty")
	}

	if groupFirstPayload, err := yakit.GetPayloadByGroupFirst(s.GetProfileDatabase(), req.GetGroup()); err != nil {
		return nil, err
	} else if groupFirstPayload.IsFile == nil && *groupFirstPayload.IsFile {
		db := s.GetProfileDatabase().Model(&yakit.Payload{})
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetIds())
		var payloads []yakit.Payload
		if err := db.Find(&payloads).Error; err != nil {
			return nil, utils.Wrap(err, "error finding payloads")
		}

		for _, payload := range payloads {
			// write to target file payload group
			if err := appendDataToFileEnd(*groupFirstPayload.Content, *payload.Content); err != nil {
				return nil, err
			} else {
				return &ypb.Empty{}, nil
			}
		}
		if !req.Copy {
			// if move to target
			// just delete original payload
			yakit.DeleteDomainByID(s.GetProfileDatabase(), req.GetIds()...)
		}
	} else {
		if req.Copy {
			// copy payloads to database
			yakit.CopyPayloads(s.GetProfileDatabase(), req.GetIds(), req.GetGroup(), req.GetFolder())
		} else {
			// move payloads to database
			yakit.MovePayloads(s.GetProfileDatabase(), req.GetIds(), req.GetGroup(), req.GetFolder())
		}
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
	if err := yakit.CreateAndUpdatePayload(s.GetProfileDatabase(), "", getEmptyFolderName(req.Name), req.Name, 0); err != nil {
		return nil, err
	} else {
		return &ypb.Empty{}, nil
	}
}

func (s *Server) UpdateAllPayloadGroup(ctx context.Context, req *ypb.UpdateAllPayloadGroupRequest) (*ypb.Empty, error) {
	nodes := req.Nodes
	folder := ""
	index := 0
	for _, node := range nodes {
		if node.Type == "Folder" {
			yakit.SetIndexToFolder(s.GetProfileDatabase(), node.Name, index)
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
		Nodes: nodes,
	}, nil
}

func (s *Server) GetAllPayload(ctx context.Context, req *ypb.GetAllPayloadRequest) (*ypb.GetAllPayloadResponse, error) {
	if req.GetGroup() == "" {
		return nil, utils.Errorf("group is empty")
	}
	db := bizhelper.ExactQueryString(s.GetProfileDatabase(), "`group`", req.GetGroup())
	db = bizhelper.ExactQueryString(db, "`folder`", req.GetFolder())

	var payloads []*ypb.Payload
	gen := yakit.YieldPayloads(db, context.Background())

	for p := range gen {
		payloads = append(payloads, Payload2Grpc(p))
	}

	return &ypb.GetAllPayloadResponse{
		Data: payloads,
	}, nil
}

func (s *Server) GetAllPayloadFromFile(req *ypb.GetAllPayloadRequest, stream ypb.Yak_GetAllPayloadFromFileServer) error {
	if req.GetGroup() == "" {
		return utils.Errorf("group is empty")
	}
	if filename, err := yakit.GetPayloadGroupFileName(s.GetProfileDatabase(), req.GetGroup()); err != nil {
		return err
	} else {
		ch, err := utils.FileLineReader(filename)
		// data, err := os.ReadFile(filename)
		if err != nil {
			return utils.Wrap(err, "read file error")
		} else {
			for line := range ch {
				stream.SendMsg(line)
			}
			return nil
		}
	}
}
