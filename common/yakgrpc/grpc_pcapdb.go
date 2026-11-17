package yakgrpc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Normal servers share the existing Yak library manager. Explicit profile
// bindings use a private manager, so tests and embedded servers stay isolated.
func (s *Server) getPCAPManager() (*pcapdb.InstanceManager, error) {
	if s == nil || s.profileDatabase == nil {
		return pcapdb.DefaultManager()
	}
	s.pcapManagerMu.Lock()
	defer s.pcapManagerMu.Unlock()
	if s.pcapManager == nil {
		base := s.cacheDir
		if base == "" {
			base = consts.GetDefaultYakitBaseDir()
		}
		manager, err := pcapdb.NewInstanceManager(s.GetProfileDatabase(), filepath.Join(base, "pcap-library"))
		if err != nil {
			return nil, err
		}
		s.pcapManager = manager
	}
	return s.pcapManager, nil
}

// ClosePCAPDatabases releases only this server's private manager, never the
// process-wide manager used by other gRPC servers and Yak executions.
func (s *Server) ClosePCAPDatabases() error {
	if s == nil {
		return nil
	}
	s.pcapManagerMu.Lock()
	manager := s.pcapManager
	s.pcapManager = nil
	s.pcapManagerMu.Unlock()
	if manager != nil {
		return manager.Close()
	}
	return nil
}

func pcapRPCError(err error) error {
	if err == nil {
		return nil
	}
	code := codes.Internal
	switch {
	case errors.Is(err, context.Canceled):
		code = codes.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	case errors.Is(err, pcapdb.ErrNotFound), errors.Is(err, os.ErrNotExist):
		code = codes.NotFound
	case errors.Is(err, pcapdb.ErrBusy):
		code = codes.Aborted
	case errors.Is(err, pcapdb.ErrNotReady), errors.Is(err, pcapdb.ErrUnsupportedSchema):
		code = codes.FailedPrecondition
	case errors.Is(err, pcapdb.ErrClosed):
		code = codes.Unavailable
	case errors.Is(err, pcapdb.ErrResultTooLarge):
		code = codes.ResourceExhausted
	case errors.Is(err, pcapdb.ErrRangeOutOfBounds):
		code = codes.OutOfRange
	case errors.Is(err, os.ErrExist):
		code = codes.AlreadyExists
	case errors.Is(err, os.ErrPermission):
		code = codes.PermissionDenied
	}
	return grpcstatus.Error(code, err.Error())
}

func pcapDatabaseModel(meta *pcapdb.PCAPFileDBMetadata) *ypb.PCAPDatabase {
	return &ypb.PCAPDatabase{
		Id: int64(meta.ID), DatasetID: meta.DatasetID, SourcePath: meta.SourcePath,
		SourceSize: meta.SourceSize, Format: meta.Format, State: meta.State,
		PacketCount: meta.PacketCount, LastError: meta.LastError, ProtocolsIndexed: meta.ProtocolsIndexed,
	}
}

func validatePCAPDatasetID(id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return grpcstatus.Error(codes.InvalidArgument, "DatasetID must be an exact canonical dataset UUID")
	}
	return nil
}

func pcapPagination(p *ypb.Paging) (*ypb.Paging, error) {
	if p.GetLimit() < 0 || p.GetLimit() > 1000 || p.GetAfterId() < 0 ||
		p.GetPage() < 0 || p.GetPage() > 1 || p.GetBeforeId() != 0 || p.GetRawOrder() != "" ||
		(p.GetOrderBy() != "" && strings.ToLower(p.GetOrderBy()) != "id") ||
		(p.GetOrder() != "" && strings.ToLower(p.GetOrder()) != "asc") {
		return nil, grpcstatus.Error(codes.InvalidArgument, "PCAP queries support Limit=1..1000 and AfterId with ascending ID order; page-number navigation is not supported")
	}
	limit := p.GetLimit()
	if limit == 0 {
		limit = 100
	}
	return &ypb.Paging{Page: 1, Limit: limit, AfterId: p.GetAfterId(), OrderBy: "id", Order: "asc"}, nil
}

func (s *Server) QueryPCAPDatabase(ctx context.Context, req *ypb.QueryPCAPDatabaseRequest) (*ypb.QueryPCAPDatabaseResponse, error) {
	paging, err := pcapPagination(req.GetPagination())
	if err != nil {
		return nil, err
	}
	id := req.GetFilter().GetDatasetID()
	if id != "" {
		if err := validatePCAPDatasetID(id); err != nil {
			return nil, err
		}
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return nil, pcapRPCError(err)
	}
	response := &ypb.QueryPCAPDatabaseResponse{
		Pagination: paging, Data: make([]*ypb.PCAPDatabase, 0), NextCursor: paging.AfterId,
	}
	if id != "" {
		meta, err := manager.MetadataContext(ctx, id)
		if errors.Is(err, pcapdb.ErrNotFound) {
			return response, nil
		}
		if err != nil {
			return nil, pcapRPCError(err)
		}
		response.Total = 1
		if int64(meta.ID) > paging.AfterId {
			response.Data = append(response.Data, pcapDatabaseModel(meta))
			response.NextCursor = int64(meta.ID)
		}
		return response, nil
	}
	page, err := manager.ListPage(ctx, pcapdb.QueryLimit(int(paging.Limit)), pcapdb.QueryAfter(paging.AfterId))
	if err != nil {
		return nil, pcapRPCError(err)
	}
	for _, row := range page.Items {
		response.Data = append(response.Data, &ypb.PCAPDatabase{
			Id: int64(row.ID), DatasetID: row.DatasetID, SourcePath: row.SourcePath,
			SourceSize: row.SourceSize, Format: row.Format, State: row.State,
			PacketCount: row.PacketCount, LastError: row.LastError, PreviewTruncated: row.PreviewTruncated,
		})
	}
	response.Total, err = manager.CountContext(ctx)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	response.NextCursor, response.HasMore, response.Truncated = page.NextCursor, page.HasMore, page.Truncated
	return response, nil
}

// Full responses remain bounded while each delivered packet stays complete.
const pcapPacketResponseBytes = 32 << 20

func pcapPacketOptions(filter *ypb.PCAPPacketFilter) ([]pcapdb.QueryOption, error) {
	if filter.GetId() < 0 {
		return nil, grpcstatus.Error(codes.InvalidArgument, "packet Id cannot be negative")
	}
	options := make([]pcapdb.QueryOption, 0, 6)
	if filter.GetTransport() != "" {
		options = append(options, pcapdb.QueryTransport(filter.GetTransport()))
	}
	for _, address := range []struct {
		value  string
		option func(string) pcapdb.QueryOption
	}{
		{filter.GetSourceIP(), pcapdb.QuerySourceIP},
		{filter.GetDestinationIP(), pcapdb.QueryDestinationIP},
	} {
		if address.value != "" {
			ip := net.ParseIP(address.value)
			if ip == nil {
				return nil, grpcstatus.Error(codes.InvalidArgument, "IP filters must be individual IPv4 or IPv6 addresses")
			}
			options = append(options, address.option(ip.String()))
		}
	}
	if filter != nil {
		for _, port := range []struct {
			value  *int32
			option func(int) pcapdb.QueryOption
		}{
			{filter.SourcePort, pcapdb.QuerySourcePort},
			{filter.DestinationPort, pcapdb.QueryDestinationPort},
		} {
			if port.value != nil {
				if *port.value < 0 || *port.value > 65535 {
					return nil, grpcstatus.Error(codes.InvalidArgument, "port filters must be 0..65535")
				}
				options = append(options, port.option(int(*port.value)))
			}
		}
	}
	if r := filter.GetTimeRange(); r != nil {
		if r.EndNS < r.StartNS {
			return nil, grpcstatus.Error(codes.InvalidArgument, "time range must satisfy StartNS <= EndNS")
		}
		options = append(options, pcapdb.QueryTimeRange(r.StartNS, r.EndNS))
	}
	return options, nil
}

func pcapPacketModel(p pcapdb.Packet) *ypb.PCAPPacket {
	return &ypb.PCAPPacket{
		Id: int64(p.ID), TimestampNS: p.TimestampNS,
		CapturedLength: int64(p.CapturedLength), OriginalLength: int64(p.OriginalLength),
		LinkType: int64(p.LinkType), Section: p.Section, Interface: int64(p.Interface),
		SourceMAC: p.SourceMAC, DestinationMAC: p.DestinationMAC, VLANStack: p.VLANStack,
		SourceIP: p.SourceIP, DestinationIP: p.DestinationIP,
		SourcePort: int32(p.SourcePort), DestinationPort: int32(p.DestinationPort),
		Transport: p.Transport, DecodeError: p.DecodeError,
	}
}

func (s *Server) QueryPCAPPacket(ctx context.Context, req *ypb.QueryPCAPPacketRequest) (*ypb.QueryPCAPPacketResponse, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	paging, err := pcapPagination(req.GetPagination())
	if err != nil {
		return nil, err
	}
	filter := req.GetFilter()
	options, err := pcapPacketOptions(filter)
	if err != nil {
		return nil, err
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return nil, pcapRPCError(err)
	}
	handle, err := manager.Acquire(ctx, req.GetDatasetID())
	if err != nil {
		return nil, pcapRPCError(err)
	}
	defer handle.Close()
	response := &ypb.QueryPCAPPacketResponse{
		Pagination: paging, Data: make([]*ypb.PCAPPacket, 0), NextCursor: paging.AfterId,
	}
	if len(options) == 0 && filter.GetId() == 0 {
		meta, err := handle.Metadata()
		if err != nil {
			return nil, pcapRPCError(err)
		}
		response.Total = proto.Int64(meta.PacketCount)
	}
	after, limit := paging.AfterId, int(paging.Limit)
	if id := filter.GetId(); id > 0 {
		if after < id-1 {
			after = id - 1
		}
		limit = 1
	}
	query := append(append([]pcapdb.QueryOption(nil), options...), pcapdb.QueryAfter(after), pcapdb.QueryLimit(limit))
	packets, err := handle.QueryPackets(query...)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	if id := filter.GetId(); id > 0 {
		if len(packets) != 0 && int64(packets[0].ID) != id {
			packets = nil
		}
		if paging.AfterId == 0 {
			response.Total = proto.Int64(int64(len(packets)))
		}
	} else if len(packets) == limit {
		// One-row lookahead also works at the native 1000-row limit.
		next := append(append([]pcapdb.QueryOption(nil), options...),
			pcapdb.QueryAfter(int64(packets[len(packets)-1].ID)), pcapdb.QueryLimit(1))
		extra, err := handle.QueryPackets(next...)
		if err != nil {
			return nil, pcapRPCError(err)
		}
		response.HasMore = len(extra) != 0
	}
	used := proto.Size(response) + 128
	for _, packet := range packets {
		item := pcapPacketModel(packet)
		size := proto.Size(item) + 16
		if req.GetFull() {
			size += packet.CapturedLength + 10
		}
		if size > pcapPacketResponseBytes-used {
			if len(response.Data) == 0 {
				return nil, grpcstatus.Error(codes.ResourceExhausted, "packet exceeds the response byte budget; use ExportPCAPData")
			}
			response.HasMore, response.Truncated = true, true
			break
		}
		if req.GetFull() {
			item.Raw, err = handle.ReadPacket(packet.ID)
			if err != nil {
				return nil, pcapRPCError(err)
			}
		}
		response.Data = append(response.Data, item)
		response.NextCursor = item.Id
		used += size
	}
	return response, nil
}

func (s *Server) ExportPCAPData(ctx context.Context, req *ypb.ExportPCAPDataRequest) (*ypb.ExportPCAPDataResponse, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetTargetPath()) == "" || req.GetPacketID() < 0 || uint64(req.GetPacketID()) > uint64(^uint(0)) {
		return nil, grpcstatus.Error(codes.InvalidArgument, "TargetPath is required and PacketID must be a nonnegative native record ID")
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return nil, pcapRPCError(err)
	}
	handle, err := manager.Acquire(ctx, req.GetDatasetID())
	if err != nil {
		return nil, pcapRPCError(err)
	}
	defer handle.Close()
	if req.GetPacketID() == 0 {
		meta, err := handle.Metadata()
		if err != nil {
			return nil, pcapRPCError(err)
		}
		path, err := handle.Export(req.GetTargetPath())
		if err != nil {
			return nil, pcapRPCError(err)
		}
		return &ypb.ExportPCAPDataResponse{Path: path, Bytes: meta.SourceSize, SHA256: meta.FullSHA256}, nil
	}
	artifact, err := handle.ExportPacket(uint(req.GetPacketID()), req.GetTargetPath())
	if err != nil {
		return nil, pcapRPCError(err)
	}
	return &ypb.ExportPCAPDataResponse{Path: artifact.Path, Bytes: artifact.Bytes, SHA256: artifact.SHA256}, nil
}

func (s *Server) DeletePCAPDatabase(ctx context.Context, req *ypb.DeletePCAPDatabaseRequest) (*ypb.DeletePCAPDatabaseResponse, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return nil, pcapRPCError(err)
	}
	result, err := manager.Delete(ctx, req.GetDatasetID())
	if err != nil && !result.Deleted {
		return nil, pcapRPCError(err)
	}
	response := &ypb.DeletePCAPDatabaseResponse{
		Message: &ypb.DbOperateMessage{
			TableName: "pcapfile_db_metadata", Operation: DbOperationDelete, EffectRows: result.AffectedRows,
		},
		Deleted: result.Deleted, CleanupPending: result.CleanupPending,
	}
	if err != nil {
		// gRPC discards a response returned with an error. Preserve a committed
		// tombstone and cleanup status as a business result that can be retried.
		response.CleanupError = err.Error()
	}
	return response, nil
}

func (s *Server) ImportPCAPFile(req *ypb.ImportPCAPFileRequest, stream ypb.Yak_ImportPCAPFileServer) error {
	if strings.TrimSpace(req.GetInputFile()) == "" {
		return grpcstatus.Error(codes.InvalidArgument, "InputFile is required")
	}
	info, err := os.Stat(req.GetInputFile())
	if err != nil {
		return pcapRPCError(err)
	}
	if !info.Mode().IsRegular() {
		return grpcstatus.Error(codes.InvalidArgument, "InputFile must be a regular capture file")
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return pcapRPCError(err)
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	var sendErr error
	var lastSent time.Time
	var lastState string
	handle, err := manager.GetOrCreateHandle(ctx, req.GetInputFile(),
		pcapdb.WithProtocols(true), pcapdb.WithStreams(false),
		pcapdb.WithProgress(func(p pcapdb.Progress) {
			if sendErr != nil {
				return
			}
			now := time.Now()
			if p.State == lastState && now.Sub(lastSent) < 100*time.Millisecond {
				return
			}
			lastSent, lastState = now, p.State
			sendErr = stream.Send(&ypb.ImportPCAPFileResponse{
				DatasetID: p.DatasetID, State: p.State, BytesIndexed: p.BytesIndexed,
				TotalBytes: p.TotalBytes, PacketCount: p.PacketCount,
			})
			if sendErr != nil {
				cancel()
			}
		}),
	)
	if sendErr != nil {
		return sendErr
	}
	if err != nil {
		return pcapRPCError(err)
	}
	defer handle.Close()
	meta, err := handle.Metadata()
	if err != nil {
		return pcapRPCError(err)
	}
	return stream.Send(&ypb.ImportPCAPFileResponse{
		DatasetID: meta.DatasetID, State: meta.State, BytesIndexed: meta.BytesIndexed,
		TotalBytes: meta.SourceSize, PacketCount: meta.PacketCount,
		Finished: true, Database: pcapDatabaseModel(meta),
	})
}
