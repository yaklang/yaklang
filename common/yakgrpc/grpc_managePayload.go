package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"time"
	"yaklang/common/cybertunnel/ctxio"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yakgrpc/yakit"
	"yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryPayload(ctx context.Context, req *ypb.QueryPayloadRequest) (*ypb.QueryPayloadResponse, error) {
	p, d, err := yakit.QueryPayload(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}

	var items []*ypb.Payload
	for _, r := range d {
		payloadContent, err := strconv.Unquote(r.Content)
		if err != nil {
			items = append(items, &ypb.Payload{
				Id:           int64(r.ID),
				Group:        r.Group,
				ContentBytes: []byte(r.Content),
				Content:      r.Content,
			})
			continue
		}
		items = append(items, &ypb.Payload{
			Id:           int64(r.ID),
			Group:        r.Group,
			ContentBytes: []byte(payloadContent),
			Content:      utils.EscapeInvalidUTF8Byte([]byte(payloadContent)),
		})
	}

	return &ypb.QueryPayloadResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
		Data:       items,
	}, nil
}

func (s *Server) DeletePayloadByGroup(ctx context.Context, req *ypb.DeletePayloadByGroupRequest) (*ypb.Empty, error) {
	if db := s.GetProfileDatabase().Model(&yakit.Payload{}).Where("`group` = ?", req.Group).Unscoped().Delete(&yakit.Payload{}); db.Error != nil {
		return nil, utils.Errorf("delete failed: %s", db.Error)
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeletePayload(ctx context.Context, req *ypb.DeletePayloadRequest) (*ypb.Empty, error) {
	if req.GetId() > 0 {
		if db := s.GetProfileDatabase().Model(&yakit.Payload{}).Where("id = ?", req.GetId()).Unscoped().Delete(&yakit.Payload{}); db.Error != nil {
			return nil, utils.Errorf("delete single line failed: %s", db.Error)
		}
	}

	if len(req.GetIds()) > 0 {
		if db := bizhelper.ExactQueryInt64ArrayOr(s.GetProfileDatabase(), "id", req.GetIds()).Unscoped().Delete(&yakit.Payload{}); db.Error != nil {
			return nil, utils.Errorf("delete mutli id failed: %s", db.Error)
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) SavePayloadStream(req *ypb.SavePayloadRequest, stream ypb.Yak_SavePayloadStreamServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	group := req.GetGroup()
	if group == "" {
		return utils.Errorf("group is empty")
	}

	var size int64
	var total int64
	start := time.Now()
	feedback := func() {
		if total <= 0 {
			total += 1
		}
		d := time.Now().Sub(start)
		stream.Send(&ypb.SavePayloadProgress{
			Progress:            float64(size) / float64(total),
			HandledBytes:        size,
			HandledBytesVerbose: utils.ByteSize(uint64(size)),
			TotalBytes:          total,
			TotalBytesVerbose:   utils.ByteSize(uint64(total)),
			CostDuration:        d.Seconds(),
			CostDurationVerbose: d.String(),
		})
	}
	go func() {
		feedback()
		defer func() {
			d := time.Now().Sub(start)
			stream.Send(&ypb.SavePayloadProgress{
				Progress:            1,
				HandledBytes:        total,
				HandledBytesVerbose: utils.ByteSize(uint64(total)),
				TotalBytes:          total,
				TotalBytesVerbose:   utils.ByteSize(uint64(total)),
				CostDuration:        d.Seconds(),
				CostDurationVerbose: d.String(),
			})
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
				feedback()
			}

		}
	}()
	handleFile := func(f string) error {
		if state, err := os.Stat(f); err != nil {
			return err
		} else {
			total += state.Size()
		}
		fp, err := os.Open(f)
		if err != nil {
			return err
		}
		defer fp.Close()

		scanner := bufio.NewScanner(ctxio.NewReader(ctx, fp))
		scanner.Split(bufio.ScanLines)

		isCSV := strings.HasSuffix(f, ".csv")
		if isCSV {
			for scanner.Scan() {
				size += int64(len(scanner.Bytes()))
				for _, p := range utils.PrettifyListFromStringSplited(scanner.Text(), ",") {
					if p == "" {
						continue
					}
					payload := &yakit.Payload{
						Group:   group,
						Content: strconv.Quote(p),
					}
					payload.Hash = payload.CalcHash()
					err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload.Hash, payload)
					if err != nil {
						log.Errorf("create or update payload error: %s", err.Error())
						continue
					}
				}
			}
		} else {
			for scanner.Scan() {
				size += int64(len(scanner.Bytes()))
				payload := &yakit.Payload{
					Group:   group,
					Content: strconv.Quote(scanner.Text()),
				}
				payload.Hash = payload.CalcHash()
				err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload.Hash, payload)
				if err != nil {
					log.Errorf("create or update payload error: %s", err.Error())
					continue
				}
			}
		}
		return nil
	}

	if req.IsFile {
		for _, f := range req.FileName {
			err := handleFile(f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	lineScanner := bufio.NewScanner(ctxio.NewReader(ctx, bytes.NewBufferString(req.GetContent())))
	total += int64(len(req.GetContent()))
	lineScanner.Split(bufio.ScanLines)

	for lineScanner.Scan() {
		size += int64(len(lineScanner.Bytes()))
		payload := &yakit.Payload{
			Group:   group,
			Content: strconv.Quote(lineScanner.Text()),
		}
		payload.Hash = payload.CalcHash()
		err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload.Hash, payload)
		if err != nil {
			log.Errorf("create or update payload error: %s", err.Error())
			continue
		}
	}
	return nil
}

func (s *Server) SavePayload(ctx context.Context, req *ypb.SavePayloadRequest) (*ypb.Empty, error) {
	group := req.GetGroup()
	if group == "" {
		return nil, utils.Errorf("group is empty")
	}

	if req.IsFile {
		for _, f := range req.FileName {
			fp, err := os.Open(f)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(fp)
			scanner.Split(bufio.ScanLines)

			for scanner.Scan() {
				if strings.HasSuffix(f, ".csv") {
					for _, p := range utils.PrettifyListFromStringSplited(scanner.Text(), ",") {
						if p == "" {
							continue
						}
						payload := &yakit.Payload{
							Group:   group,
							Content: strconv.Quote(p),
						}
						payload.Hash = payload.CalcHash()
						err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload.Hash, payload)
						if err != nil {
							log.Errorf("create or update payload error: %s", err.Error())
							continue
						}
					}
				} else {
					payload := &yakit.Payload{
						Group:   group,
						Content: strconv.Quote(scanner.Text()),
					}
					payload.Hash = payload.CalcHash()
					err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload.Hash, payload)
					if err != nil {
						log.Errorf("create or update payload error: %s", err.Error())
						continue
					}
				}

			}
		}
		return &ypb.Empty{}, nil
	}

	lineScanner := bufio.NewScanner(bytes.NewBufferString(req.GetContent()))
	lineScanner.Split(bufio.ScanLines)

	for lineScanner.Scan() {
		payload := &yakit.Payload{
			Group:   group,
			Content: strconv.Quote(lineScanner.Text()),
		}
		payload.Hash = payload.CalcHash()
		err := yakit.CreateOrUpdatePayload(s.GetProfileDatabase(), payload.Hash, payload)
		if err != nil {
			log.Errorf("create or update payload error: %s", err.Error())
			continue
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) GetAllPayloadGroup(ctx context.Context, _ *ypb.Empty) (*ypb.GetAllPayloadGroupResponse, error) {
	var res []struct {
		Group string
	}
	if db := s.GetProfileDatabase().Raw(`select distinct(payloads."group") from payloads;`).Scan(&res); db.Error != nil {
		return nil, db.Error
	}

	var results []string
	for _, r := range res {
		results = append(results, r.Group)
	}
	return &ypb.GetAllPayloadGroupResponse{Groups: results}, nil
}

func (s *Server) UpdatePayload(ctx context.Context, req *ypb.UpdatePayloadRequest) (*ypb.Empty, error) {
	if req.GetGroup() == "" || req.GetOldGroup() == "" {
		return nil, utils.Errorf("group is empty")
	}
	err := yakit.UpdatePayload(s.GetProfileDatabase(), req)
	if err != nil {
		log.Errorf("update payload error: %s", err.Error())
		return nil, utils.Errorf("update failed: %v", err.Error())
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetAllPayload(ctx context.Context, req *ypb.GetAllPayloadRequest) (*ypb.GetAllPayloadResponse, error) {
	if req.GetGroup() == "" {
		return nil, utils.Errorf("group is empty")
	}
	db := bizhelper.ExactQueryString(s.GetProfileDatabase(), "`group`", req.GetGroup())
	var payloads []*ypb.Payload
	gen := yakit.YieldPayloads(db, context.Background())

	for p := range gen {
		raw, err := strconv.Unquote(p.Content)
		if err != nil {
			payloads = append(payloads, &ypb.Payload{
				Content: p.Content,
			})

			continue
		}
		payloads = append(payloads, &ypb.Payload{
			Content: raw,
		})
	}

	return &ypb.GetAllPayloadResponse{
		Data: payloads,
	}, nil
}
