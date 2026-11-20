package yakgrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	pcapDetailSourceInitialBytes = 64 << 10
	pcapDetailInitialBudget      = 256 << 10
	pcapDetailMessagesBudget     = 8 << 20
)

func pcapDisplayFieldNodes(sourceID string, nodes []pcaputil.DisplayNode) []*ypb.PCAPFieldNode {
	result := make([]*ypb.PCAPFieldNode, 0, len(nodes))
	for _, node := range nodes {
		item := &ypb.PCAPFieldNode{
			Name:     node.Name,
			Children: pcapDisplayFieldNodes(sourceID, node.Children),
			Ranges:   make([]*ypb.PCAPFieldRange, 0, len(node.Ranges)),
		}
		if node.DisplayValue != "" {
			item.DisplayValue = proto.String(node.DisplayValue)
		}
		for _, byteRange := range node.Ranges {
			if byteRange.BitOffset < 0 || byteRange.BitLength <= 0 {
				continue
			}
			item.Ranges = append(item.Ranges, &ypb.PCAPFieldRange{
				SourceID: sourceID, BitOffset: byteRange.BitOffset, BitLength: byteRange.BitLength,
			})
		}
		result = append(result, item)
	}
	return result
}

func pcapInitialRange(handle *pcapdb.DatabaseHandle, protocol bool, id uint, length int64, remaining *int64) ([]byte, bool, error) {
	if length <= 0 || *remaining <= 0 {
		return nil, length > 0, nil
	}
	readLength := length
	if readLength > int64(pcapDetailSourceInitialBytes) {
		readLength = int64(pcapDetailSourceInitialBytes)
	}
	if readLength > *remaining {
		readLength = *remaining
	}
	if readLength <= 0 {
		return nil, true, nil
	}
	var result *pcapdb.DataRange
	var err error
	if protocol {
		result, err = handle.ReadProtocolRange(id, 0, int(readLength))
	} else {
		result, err = handle.ReadPacketRange(id, 0, int(readLength))
	}
	if err != nil {
		return nil, true, err
	}
	*remaining -= int64(len(result.Data))
	return result.Data, int64(len(result.Data)) < length, nil
}

// A previous PDU can exhaust the bytes sent to the client. Its successor may
// still need a few header bytes to locate fields. Read only the missing prefix,
// within a separate per-request budget; don't enlarge ByteSource.InitialData.
func pcapMessageProjection(handle *pcapdb.DatabaseHandle, message pcapdb.ProtocolMessage, source pcaputil.ByteSource, initial []byte, remaining *int) (pcaputil.MessageDisplayInput, error) {
	m := pcaputil.MessageDisplayInput{Protocol: message.Protocol, Profile: message.Profile, Status: message.Status,
		Transport: message.Transport, Source: source, Prefix: initial, Length: message.DataLength}
	needed := min(m.WirePrefixLength(), pcapDetailSourceInitialBytes)
	if n := min(needed-len(initial), *remaining); n > 0 {
		extra, err := handle.ReadProtocolRange(message.ID, int64(len(initial)), n)
		if err != nil {
			return m, err
		}
		*remaining -= len(extra.Data)
		if len(initial) == 0 {
			m.Prefix = extra.Data
		} else {
			m.Prefix = append(initial[:len(initial):len(initial)], extra.Data...)
		}
	}
	return m, nil
}

func pcapBuildMessageNodes(message pcapdb.ProtocolMessage, sourceID string, view pcaputil.MessageDisplayInput) ([]*ypb.PCAPFieldNode, bool) {
	view.Fields, view.Session = message.Fields, message.Session
	projected, truncated := pcaputil.BuildMessageDisplayNodes(view)
	return pcapDisplayFieldNodes(sourceID, projected), truncated
}

func pcapFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) GetPCAPPacketDetail(ctx context.Context, req *ypb.GetPCAPPacketDetailRequest) (*ypb.PCAPPacketDetail, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	if req.GetPacketID() <= 0 || uint64(req.GetPacketID()) > uint64(^uint(0)) {
		return nil, grpcstatus.Error(codes.InvalidArgument, "PacketID must be a positive native record ID")
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
	packetID := uint(req.GetPacketID())
	page, err := handle.QueryPacketView(pcapdb.PacketViewFilter{ID: int64(packetID), Limit: 1, SkipTotal: true})
	if err != nil {
		return nil, pcapRPCError(err)
	}
	if len(page.Packets) != 1 {
		return nil, pcapRPCError(pcapdb.ErrNotFound)
	}
	row := page.Packets[0]
	raw, err := handle.ReadPacket(packetID)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	frameSourceID := fmt.Sprintf("frame:%d", packetID)
	response := &ypb.PCAPPacketDetail{
		Packet: pcapPacketSummaryModel(row),
		Nodes: pcapDisplayFieldNodes(frameSourceID, pcaputil.BuildFrameDisplayNodes(pcaputil.FrameDisplayInput{
			Raw: raw, LinkType: row.Packet.LinkType, PacketID: uint64(row.Packet.ID),
			CapturedLength: row.Packet.CapturedLength, OriginalLength: row.Packet.OriginalLength,
			Section: row.Packet.Section, Interface: row.Packet.Interface, TimestampNS: row.Packet.TimestampNS,
		})),
		Messages:        make([]*ypb.PCAPMessageDetail, 0),
		RelatedMessages: make([]*ypb.PCAPRelatedMessage, 0),
		ByteSources:     make([]*ypb.PCAPByteSource, 0),
	}
	remainingInitial := int64(pcapDetailInitialBudget)
	remainingProjection := pcapDetailInitialBudget
	frameInitial, frameTruncated, err := pcapInitialRange(handle, false, packetID, int64(len(raw)), &remainingInitial)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	response.ByteSources = append(response.ByteSources, &ypb.PCAPByteSource{
		ID: frameSourceID, Kind: "frame", Name: fmt.Sprintf("Frame #%d", packetID),
		Length: int64(len(raw)), Completeness: "frame", InitialData: frameInitial,
		InitialOffset: 0, InitialTruncated: frameTruncated,
	})
	completed, related, err := handle.PacketDetailMessages(packetID, pcapDetailMessagesBudget)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	for _, message := range completed {
		sourceID := fmt.Sprintf("message:%d", message.ID)
		var decodeDiagnostics []string
		source, err := pcapdb.MessageByteSource(message)
		if err != nil {
			decodeDiagnostics = append(decodeDiagnostics, "source bytes: "+err.Error())
		}
		initial, initialTruncated, err := pcapInitialRange(handle, true, message.ID, int64(message.DataLength), &remainingInitial)
		if err != nil {
			return nil, pcapRPCError(err)
		}
		view, err := pcapMessageProjection(handle, message, source, initial, &remainingProjection)
		if err != nil {
			return nil, pcapRPCError(err)
		}
		nodes, nodesTruncated := pcapBuildMessageNodes(message, sourceID, view)
		completedIn := uint64(message.CompletedInPacketID)
		if completedIn == 0 && source.CompletedIn != nil {
			completedIn = source.CompletedIn.Number
		}
		contributors := make([]uint64, 0, len(source.PacketRefs))
		for _, reference := range source.PacketRefs {
			contributors = append(contributors, reference.Number)
		}
		response.Messages = append(response.Messages, &ypb.PCAPMessageDetail{
			MessageId: uint64(message.ID), Protocol: strings.ToUpper(message.Protocol),
			Summary: message.Summary, Status: message.Status, Completeness: message.Completeness,
			SourceID: sourceID, Nodes: nodes,
			DecodeError:    pcapFirstNonEmpty(message.DecodeError, message.Error, strings.Join(decodeDiagnostics, "; ")),
			NodesTruncated: nodesTruncated,
		})
		response.ByteSources = append(response.ByteSources, &ypb.PCAPByteSource{
			ID: sourceID, Kind: "message",
			Name:   fmt.Sprintf("%s reassembled message #%d", strings.ToUpper(message.Protocol), message.ID),
			Length: int64(message.DataLength), Completeness: message.Completeness,
			ContributorPacketIds: contributors, CompletedInPacketId: completedIn,
			InitialData: initial, InitialOffset: 0, InitialTruncated: initialTruncated,
		})
	}
	for _, message := range related {
		completedIn := uint64(message.CompletedInPacketID)
		if completedIn == 0 {
			source, sourceErr := pcapdb.MessageByteSource(message)
			if sourceErr == nil && source.CompletedIn != nil {
				completedIn = source.CompletedIn.Number
			}
		}
		response.RelatedMessages = append(response.RelatedMessages, &ypb.PCAPRelatedMessage{
			MessageId: uint64(message.ID), Protocol: strings.ToUpper(message.Protocol),
			Summary: message.Summary, SourceID: fmt.Sprintf("message:%d", message.ID),
			CompletedInPacketId: completedIn, Completeness: message.Completeness,
		})
	}
	return response, nil
}
