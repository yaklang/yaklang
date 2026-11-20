package yakgrpc

import (
	"context"
	"errors"
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
	createdAt := int64(0)
	if !meta.CreatedAt.IsZero() {
		createdAt = meta.CreatedAt.Unix()
	}
	return &ypb.PCAPDatabase{
		Id: int64(meta.ID), DatasetID: meta.DatasetID, SourcePath: meta.SourcePath,
		SourceSize: meta.SourceSize, Format: meta.Format, State: meta.State,
		PacketCount: meta.PacketCount, LastError: meta.LastError,
		ProtocolsIndexed: meta.ProtocolsIndexed, SourceKind: meta.SourceKind,
		CaptureInterface: meta.CaptureInterface, CaptureBPF: meta.CaptureBPF,
		ProtocolCount: meta.ProtocolCount, SessionCount: meta.SessionCount, StreamCount: meta.StreamCount,
		StreamChunkCount: meta.StreamChunkCount, StreamsIndexed: meta.StreamsIndexed,
		CreatedAt: createdAt,
		PreviewTruncated: len(meta.SourcePath) > 512 || len(meta.DatabasePath) > 512 ||
			len(meta.CaptureInterface) > 512 || len(meta.CaptureBPF) > 512 || len(meta.LastError) > 512,
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

// Full responses remain bounded while each delivered packet stays complete.
const pcapPacketResponseBytes = 32 << 20

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
	page, err := manager.QueryMetadata(ctx, id, pcapdb.QueryLimit(int(paging.Limit)), pcapdb.QueryAfter(paging.AfterId))
	if err != nil {
		return nil, pcapRPCError(err)
	}
	for i := range page.Items {
		response.Data = append(response.Data, pcapDatabaseModel(&page.Items[i]))
	}
	response.Total, err = manager.CountMetadata(ctx, id)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	response.NextCursor, response.HasMore, response.Truncated = page.NextCursor, page.HasMore, page.Truncated
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
