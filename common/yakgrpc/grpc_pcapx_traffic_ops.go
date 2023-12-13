package yakgrpc

import (
	"bytes"
	"context"
	"github.com/samber/lo"
	bin_parser "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
)

func (s *Server) QueryTrafficPacket(ctx context.Context, req *ypb.QueryTrafficPacketRequest) (*ypb.QueryTrafficPacketResponse, error) {
	pg, data, err := yakit.QueryTrafficPacket(consts.GetGormProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	rspData := lo.Map(data, func(item *yakit.TrafficPacket, index int) *ypb.TrafficPacket {
		payloadBytes, _ := strconv.Unquote(item.Payload)
		raw, _ := strconv.Unquote(item.QuotedRaw)
		return &ypb.TrafficPacket{
			LinkLayerType:                   item.LinkLayerType,
			NetworkLayerType:                item.NetworkLayerType,
			TransportLayerType:              item.TransportLayerType,
			ApplicationLayerType:            item.ApplicationLayerType,
			Payload:                         []byte(payloadBytes),
			Raw:                             []byte(raw),
			EthernetEndpointHardwareAddrSrc: item.EthernetEndpointHardwareAddrSrc,
			EthernetEndpointHardwareAddrDst: item.EthernetEndpointHardwareAddrDst,
			IsIpv4:                          item.IsIpv4,
			IsIpv6:                          item.IsIpv6,
			NetworkEndpointIPSrc:            item.NetworkEndpointIPSrc,
			NetworkEndpointIPDst:            item.NetworkEndpointIPDst,
			TransportEndpointPortSrc:        int64(item.TransportEndpointPortSrc),
			TransportEndpointPortDst:        int64(item.TransportEndpointPortDst),
			SessionId:                       item.SessionUuid,
			Id:                              int64(item.ID),
		}
	})

	return &ypb.QueryTrafficPacketResponse{
		Data:       rspData,
		Pagination: req.GetPagination(),
		Total:      int64(pg.TotalRecord),
	}, nil
}

func (s *Server) QueryTrafficTCPReassembled(ctx context.Context, req *ypb.QueryTrafficTCPReassembledRequest) (*ypb.QueryTrafficTCPReassembledResponse, error) {
	pg, data, err := yakit.QueryTrafficTCPReassembled(consts.GetGormProjectDatabase(), req)
	if err != nil {
		log.Infof("query traffic tcp reassembled failed: %s", err)
		return nil, err
	}
	rspData := lo.Map(data, func(item *yakit.TrafficTCPReassembledFrame, index int) *ypb.TrafficTCPReassembled {
		var infoMap map[string]any
		if item.SerializedResults == "" {
			data := codec.StrConvUnquoteForce(item.QuotedData)
			node, err := bin_parser.ParseBinary(bytes.NewReader(data), "application_layer.tcp")
			if err != nil {
				log.Errorf("parse binary failed: %s", err)
			} else {
				res, err := node.Result()
				if err != nil {
					log.Errorf("get result failed: %s", err)
				} else {
					r, ok := res.(map[string]any)
					if ok {
						infoMap = r
					} else {
						log.Errorf("result type error")
					}
				}
			}
		} else {
			data, err := codec.DecodeBase64(item.SerializedResults)
			if err != nil {
				log.Errorf("decode base64 failed: %s", err)
			} else {
				node, err := bin_parser.ParseBinary(bytes.NewReader(data), "tcp_reassembled_parsed_result")
				if err != nil {
					log.Errorf("parse binary failed: %s", err)
				} else {
					res, err := node.Result()
					if err != nil {
						log.Errorf("get result failed: %s", err)
					} else {
						r, ok := res.(map[string]any)
						if ok {
							infoMap = r
						} else {
							log.Errorf("result type error")
						}
					}
				}
			}
		}
		if infoMap == nil {
			log.Errorf("parser reassembled frame failed, id: %d", item.ID)
		}
		source := utils.InterfaceToString(infoMap["source"])
		destination := utils.InterfaceToString(infoMap["destination"])
		protocol := utils.InterfaceToString(infoMap["protocol"])
		return &ypb.TrafficTCPReassembled{
			Id:          int64(item.ID),
			SessionUuid: item.SessionUuid,
			Raw:         codec.StrConvUnquoteForce(item.QuotedData),
			Seq:         int64(item.Seq),
			Timestamp:   int64(item.Timestamp),
			Source:      source,
			Destination: destination,
			Protocol:    protocol,
		}
	})
	return &ypb.QueryTrafficTCPReassembledResponse{
		Data:       rspData,
		Pagination: req.GetPagination(),
		Total:      int64(pg.TotalRecord),
	}, nil
}

func (s *Server) QueryTrafficSession(ctx context.Context, req *ypb.QueryTrafficSessionRequest) (*ypb.QueryTrafficSessionResponse, error) {
	log.Infof("query traffic session from id: %v", req.GetFromId())
	pg, data, err := yakit.QueryTrafficSession(consts.GetGormProjectDatabase(), req)
	if err != nil {
		log.Infof("query traffic session failed: %s", err)
		return nil, err
	}

	rspData := lo.Map(data, func(item *yakit.TrafficSession, index int) *ypb.TrafficSession {
		return &ypb.TrafficSession{
			Id:                    int64(item.ID),
			SessionType:           item.SessionType,
			Uuid:                  item.Uuid,
			DeviceName:            item.DeviceName,
			DeviceType:            item.DeviceType,
			IsLinkLayerEthernet:   item.IsLinkLayerEthernet,
			LinkLayerSrc:          item.LinkLayerSrc,
			LinkLayerDst:          item.LinkLayerDst,
			IsIpv4:                item.IsIpv4,
			IsIpv6:                item.IsIpv6,
			NetworkSrcIP:          item.NetworkSrcIP,
			NetworkDstIP:          item.NetworkDstIP,
			IsTcpIpStack:          item.IsTcpIpStack,
			TransportLayerSrcPort: int64(item.TransportLayerSrcPort),
			TransportLayerDstPort: int64(item.TransportLayerDstPort),
			IsTCPReassembled:      item.IsTCPReassembled,
			IsHalfOpen:            item.IsHalfOpen,
			IsClosed:              item.IsClosed,
			IsForceClosed:         item.IsForceClosed,
			HaveClientHello:       item.HaveClientHello,
			SNI:                   item.SNI,
		}
	})
	return &ypb.QueryTrafficSessionResponse{
		Data:       rspData,
		Pagination: req.GetPagination(),
		Total:      int64(pg.TotalRecord),
	}, nil
}
