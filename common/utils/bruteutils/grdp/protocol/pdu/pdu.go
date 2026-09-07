package pdu

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/emission"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/t125/gcc"
)

type PDULayer struct {
	emission.Emitter
	transport          core.Transport
	sharedId           uint32
	userId             uint16
	channelId          uint16
	serverCapabilities map[CapsType]Capability
	clientCapabilities map[CapsType]Capability
	fastPathSender     core.FastPathSender
	demandActivePDU    *DemandActivePDU
}

func NewPDULayer(t core.Transport) *PDULayer {
	p := &PDULayer{
		Emitter:   *emission.NewEmitter(),
		transport: t,
		sharedId:  0x103EA,
		serverCapabilities: map[CapsType]Capability{
			CAPSTYPE_GENERAL: &GeneralCapability{
				ProtocolVersion: 0x0200,
			},
			CAPSTYPE_BITMAP: &BitmapCapability{
				Receive1BitPerPixel:      0x0001,
				Receive4BitsPerPixel:     0x0001,
				Receive8BitsPerPixel:     0x0001,
				BitmapCompressionFlag:    0x0001,
				MultipleRectangleSupport: 0x0001,
			},
			CAPSTYPE_ORDER: &OrderCapability{
				DesktopSaveXGranularity: 1,
				DesktopSaveYGranularity: 20,
				MaximumOrderLevel:       1,
				OrderFlags:              NEGOTIATEORDERSUPPORT,
				DesktopSaveSize:         480 * 480,
			},
			CAPSTYPE_POINTER:        &PointerCapability{ColorPointerCacheSize: 20},
			CAPSTYPE_INPUT:          &InputCapability{},
			CAPSTYPE_VIRTUALCHANNEL: &VirtualChannelCapability{},
			CAPSTYPE_FONT:           &FontCapability{SupportFlags: 0x0001},
			CAPSTYPE_COLORCACHE:     &ColorCacheCapability{CacheSize: 0x0006},
			CAPSTYPE_SHARE:          &ShareCapability{},
		},
		clientCapabilities: map[CapsType]Capability{
			CAPSTYPE_GENERAL: &GeneralCapability{
				ProtocolVersion: 0x0200,
			},
			CAPSTYPE_BITMAP: &BitmapCapability{
				Receive1BitPerPixel:      0x0001,
				Receive4BitsPerPixel:     0x0001,
				Receive8BitsPerPixel:     0x0001,
				BitmapCompressionFlag:    0x0001,
				MultipleRectangleSupport: 0x0001,
			},
			CAPSTYPE_ORDER: &OrderCapability{
				DesktopSaveXGranularity: 1,
				DesktopSaveYGranularity: 20,
				MaximumOrderLevel:       1,
				OrderFlags:              NEGOTIATEORDERSUPPORT,
				DesktopSaveSize:         480 * 480,
			},
			CAPSTYPE_BITMAPCACHE:           &BitmapCacheCapability{},
			CAPSTYPE_POINTER:               &PointerCapability{ColorPointerCacheSize: 20},
			CAPSTYPE_INPUT:                 &InputCapability{},
			CAPSTYPE_BRUSH:                 &BrushCapability{},
			CAPSTYPE_GLYPHCACHE:            &GlyphCapability{},
			CAPSTYPE_OFFSCREENCACHE:        &OffscreenBitmapCacheCapability{},
			CAPSTYPE_VIRTUALCHANNEL:        &VirtualChannelCapability{},
			CAPSTYPE_SOUND:                 &SoundCapability{},
			CAPSETTYPE_MULTIFRAGMENTUPDATE: &MultiFragmentUpdate{},
			CAPSTYPE_RAIL: &RemoteProgramsCapability{
				RailSupportLevel: RAIL_LEVEL_SUPPORTED |
					RAIL_LEVEL_SHELL_INTEGRATION_SUPPORTED |
					RAIL_LEVEL_LANGUAGE_IME_SYNC_SUPPORTED |
					RAIL_LEVEL_SERVER_TO_CLIENT_IME_SYNC_SUPPORTED |
					RAIL_LEVEL_HIDE_MINIMIZED_APPS_SUPPORTED |
					RAIL_LEVEL_WINDOW_CLOAKING_SUPPORTED |
					RAIL_LEVEL_HANDSHAKE_EX_SUPPORTED |
					RAIL_LEVEL_DOCKED_LANGBAR_SUPPORTED,
			},
		},
	}

	t.On("close", func() {
		p.Emit("close")
	}).On("error", func(err error) {
		p.Emit("error", err)
	})
	return p
}

func (p *PDULayer) sendPDU(message PDUMessage) {
	pdu := NewPDU(p.userId, message)
	p.transport.Write(pdu.serialize())
}

func (p *PDULayer) sendDataPDU(message DataPDUData) {
	dataPdu := NewDataPDU(message, p.sharedId)
	p.sendPDU(dataPdu)
}

func (p *PDULayer) SetFastPathSender(f core.FastPathSender) {
	p.fastPathSender = f
}

type Client struct {
	*PDULayer
	clientCoreData *gcc.ClientCoreData
	remoteAppMode  bool
	enableCliprdr  bool
	// afterFontMap：第一条连接序列已走完。此后再出现 Demand Active /
	// Deactivate All 是 AUTOLOGON 成功后的会话切换（真机 XP 不发 0x26）。
	afterFontMap bool
}

func NewClient(t core.Transport) *Client {
	c := &Client{
		PDULayer: NewPDULayer(t),
	}
	c.transport.Once("connect", c.connect)
	return c
}

func (c *Client) connect(data *gcc.ClientCoreData, userId uint16, channelId uint16) {
	glog.Debug("pdu connect:", userId, ",", channelId)
	c.clientCoreData = data
	c.userId = userId
	c.channelId = channelId
	c.transport.Once("data", c.recvDemandActivePDU)
}

func (c *Client) recvDemandActivePDU(s []byte) {
	glog.Debug("PDU recvDemandActivePDU", hex.EncodeToString(s))
	r := bytes.NewReader(s)
	pdu, err := readPDU(r)
	if err != nil {
		glog.Error(err)
		return
	}
	if pdu.ShareCtrlHeader.PDUType != PDUTYPE_DEMANDACTIVEPDU {
		glog.Info("PDU ignore message during connection sequence, type is", pdu.ShareCtrlHeader.PDUType)
		c.transport.Once("data", c.recvDemandActivePDU)
		return
	}
	c.sharedId = pdu.Message.(*DemandActivePDU).SharedId
	c.demandActivePDU = pdu.Message.(*DemandActivePDU)
	for _, caps := range pdu.Message.(*DemandActivePDU).CapabilitySets {
		c.serverCapabilities[caps.Type()] = caps
	}

	c.sendConfirmActivePDU()
	c.sendClientFinalizeSynchronizePDU()
	c.transport.Once("data", c.recvServerSynchronizePDU)
}

func (c *Client) sendConfirmActivePDU() {
	glog.Debug("PDU start sendConfirmActivePDU")

	pdu := NewConfirmActivePDU()

	generalCapa := c.clientCapabilities[CAPSTYPE_GENERAL].(*GeneralCapability)
	generalCapa.OSMajorType = OSMAJORTYPE_WINDOWS
	generalCapa.OSMinorType = OSMINORTYPE_WINDOWS_NT
	generalCapa.ExtraFlags = NO_BITMAP_COMPRESSION_HDR
	// XP/2003（PROTOCOL_RDP）不支持 LONG_CREDENTIALS / salted checksum /
	// fast-path 输出。广告这些会让服务端走 fast-path 图形，SAVE_SESSION_INFO
	// 仍走慢路径，但 5.1 自动登录更稳妥的是关掉。
	if c.clientCoreData != nil && c.clientCoreData.ServerSelectedProtocol != 0 {
		generalCapa.ExtraFlags |= LONG_CREDENTIALS_SUPPORTED | ENC_SALTED_CHECKSUM | FASTPATH_OUTPUT_SUPPORTED
	}

	bitmapCapa := c.clientCapabilities[CAPSTYPE_BITMAP].(*BitmapCapability)
	bitmapCapa.PreferredBitsPerPixel = c.clientCoreData.HighColorDepth
	bitmapCapa.DesktopWidth = c.clientCoreData.DesktopWidth
	bitmapCapa.DesktopHeight = c.clientCoreData.DesktopHeight

	orderCapa := c.clientCapabilities[CAPSTYPE_ORDER].(*OrderCapability)
	orderCapa.OrderFlags |= ZEROBOUNDSDELTASSUPPORT

	inputCapa := c.clientCapabilities[CAPSTYPE_INPUT].(*InputCapability)
	inputCapa.Flags = INPUT_FLAG_SCANCODES | INPUT_FLAG_MOUSEX | INPUT_FLAG_UNICODE
	inputCapa.KeyboardLayout = c.clientCoreData.KbdLayout
	inputCapa.KeyboardType = c.clientCoreData.KeyboardType
	inputCapa.KeyboardSubType = c.clientCoreData.KeyboardSubType
	inputCapa.KeyboardFunctionKey = c.clientCoreData.KeyboardFnKeys
	inputCapa.ImeFileName = c.clientCoreData.ImeFileName

	pdu.SharedId = c.sharedId
	pdu.NumberCapabilities = c.demandActivePDU.NumberCapabilities
	for _, v := range c.clientCapabilities {
		glog.Debugf("clientCapabilities: 0x%04x", v.Type())
		pdu.CapabilitySets = append(pdu.CapabilitySets, v)
	}
	if c.remoteAppMode {
		pdu.CapabilitySets = append(pdu.CapabilitySets, c.serverCapabilities[CAPSTYPE_RAIL])
		pdu.CapabilitySets = append(pdu.CapabilitySets, c.serverCapabilities[CAPSTYPE_WINDOW])
	}
	pdu.LengthSourceDescriptor = c.demandActivePDU.LengthSourceDescriptor
	pdu.SourceDescriptor = c.demandActivePDU.SourceDescriptor
	pdu.LengthCombinedCapabilities = c.demandActivePDU.LengthCombinedCapabilities

	c.sendPDU(pdu)
}

func (c *Client) sendClientFinalizeSynchronizePDU() {
	glog.Debug("PDU start sendClientFinalizeSynchronizePDU")
	c.sendDataPDU(NewSynchronizeDataPDU(c.channelId))
	c.sendDataPDU(&ControlDataPDU{Action: CTRLACTION_COOPERATE})
	c.sendDataPDU(&ControlDataPDU{Action: CTRLACTION_REQUEST_CONTROL})
	//c.sendDataPDU(&PersistKeyPDU{BBitMask: 0x03})
	c.sendDataPDU(&FontListDataPDU{ListFlags: 0x0003, EntrySize: 0x0032})
}

func (c *Client) recvServerSynchronizePDU(s []byte) {
	glog.Debug("PDU recvServerSynchronizePDU")
	r := bytes.NewReader(s)
	pdu, err := readPDU(r)
	if err != nil {
		glog.Error(err)
		return
	}
	dataPdu, ok := pdu.Message.(*DataPDU)
	if !ok || dataPdu.Header.PDUType2 != PDUTYPE2_SYNCHRONIZE {
		if ok {
			glog.Error("recvServerSynchronizePDU ignore datapdu type2", dataPdu.Header.PDUType2)
		} else {
			glog.Error("recvServerSynchronizePDU ignore message type", pdu.ShareCtrlHeader.PDUType)
		}
		glog.Infof("%+v", dataPdu)
		c.transport.Once("data", c.recvServerSynchronizePDU)
		return
	}
	c.transport.Once("data", c.recvServerControlCooperatePDU)
}

func (c *Client) recvServerControlCooperatePDU(s []byte) {
	glog.Debug("PDU recvServerControlCooperatePDU")
	r := bytes.NewReader(s)
	pdu, err := readPDU(r)
	if err != nil {
		glog.Error(err)
		return
	}
	dataPdu, ok := pdu.Message.(*DataPDU)
	if !ok || dataPdu.Header.PDUType2 != PDUTYPE2_CONTROL {
		if ok {
			glog.Error("recvServerControlCooperatePDU ignore datapdu type2", dataPdu.Header.PDUType2)
		} else {
			glog.Error("recvServerControlCooperatePDU ignore message type", pdu.ShareCtrlHeader.PDUType)
		}
		c.transport.Once("data", c.recvServerControlCooperatePDU)
		return
	}
	if dataPdu.Data.(*ControlDataPDU).Action != CTRLACTION_COOPERATE {
		glog.Error("recvServerControlCooperatePDU ignore action", dataPdu.Data.(*ControlDataPDU).Action)
		c.transport.Once("data", c.recvServerControlCooperatePDU)
		return
	}
	c.transport.Once("data", c.recvServerControlGrantedPDU)
}

func (c *Client) recvServerControlGrantedPDU(s []byte) {
	glog.Debug("PDU recvServerControlGrantedPDU")
	r := bytes.NewReader(s)
	pdu, err := readPDU(r)
	if err != nil {
		glog.Error(err)
		return
	}
	dataPdu, ok := pdu.Message.(*DataPDU)
	if !ok || dataPdu.Header.PDUType2 != PDUTYPE2_CONTROL {
		if ok {
			glog.Error("recvServerControlGrantedPDU ignore datapdu type2", dataPdu.Header.PDUType2)
		} else {
			glog.Error("recvServerControlGrantedPDU ignore message type", pdu.ShareCtrlHeader.PDUType)
		}
		c.transport.Once("data", c.recvServerControlGrantedPDU)
		return
	}
	if dataPdu.Data.(*ControlDataPDU).Action != CTRLACTION_GRANTED_CONTROL {
		glog.Error("recvServerControlGrantedPDU ignore action", dataPdu.Data.(*ControlDataPDU).Action)
		c.transport.Once("data", c.recvServerControlGrantedPDU)
		return
	}
	c.transport.Once("data", c.recvServerFontMapPDU)
}

func (c *Client) recvServerFontMapPDU(s []byte) {
	glog.Debug("PDU recvServerFontMapPDU")
	r := bytes.NewReader(s)
	pdu, err := readPDU(r)
	if err != nil {
		glog.Error(err)
		return
	}
	dataPdu, ok := pdu.Message.(*DataPDU)
	if !ok || dataPdu.Header.PDUType2 != PDUTYPE2_FONTMAP {
		if ok {
			glog.Error("recvServerFontMapPDU ignore datapdu type2", dataPdu.Header.PDUType2)
		} else {
			glog.Error("recvServerFontMapPDU ignore message type", pdu.ShareCtrlHeader.PDUType)
		}
		c.transport.Once("data", c.recvServerFontMapPDU)
		return
	}
	c.transport.On("data", c.recvPDU)
	c.afterFontMap = true
	c.Emit("ready")
}

func (c *Client) recvPDU(s []byte) {
	glog.Debug("PDU recvPDU", hex.EncodeToString(s))
	if os.Getenv("YAK_RDP_XP_DUMP") != "" {
		dumpSharePDUs(s)
	}
	r := bytes.NewReader(s)
	for r.Len() > 0 {
		left := r.Len()
		if c.afterFontMap && classicShareRestart(remainingShareType(s, left)) {
			glog.Info("classic share restart after FontMap (AUTOLOGON)")
			c.Emit("logon", &SaveSessionInfo{InfoType: INFOTYPE_LOGON_PLAINNOTIFY})
			return
		}
		p, err := readPDU(r)
		if err != nil {
			glog.Error(err)
			if os.Getenv("YAK_RDP_XP_DUMP") != "" && left <= len(s) {
				n := left
				if n > 24 {
					n = 24
				}
				glog.Infof("readPDU EOF leftover=%d %s", left, hex.EncodeToString(s[len(s)-left:len(s)-left+n]))
			}
			return
		}
		if !c.afterFontMap && p.ShareCtrlHeader.PDUType == PDUTYPE_DEACTIVATEALLPDU {
			c.transport.On("data", c.recvDemandActivePDU)
		}
		dpdu, ok := p.Message.(*DataPDU)
		if !ok {
			continue
		}
		glog.Debugf("pdu type2=0x%02x", dpdu.Header.PDUType2)
		switch dpdu.Header.PDUType2 {
		case PDUTYPE2_SAVE_SESSION_INFO:
			if si, ok2 := dpdu.Data.(*SaveSessionInfo); ok2 {
				glog.Infof("save session info type=%d fields=%d", si.InfoType, si.FieldsPresent)
				c.Emit("logon", si)
			}
		case PDUTYPE2_SET_ERROR_INFO_PDU:
			if ei, ok2 := dpdu.Data.(*ErrorInfoDataPDU); ok2 && ei.IsLogonFailure() {
				c.Emit("error", fmt.Errorf("rdp logon failed: error info 0x%x", ei.ErrorInfo))
			}
		case PDUTYPE2_UPDATE:
			// ncrack：XP 失败/占用会话走绘图订单，不一定发 0x26。
			if sk, ok2 := dpdu.Data.(*skippedDataPDU); ok2 {
				if os.Getenv("YAK_RDP_XP_DUMP") != "" && len(sk.raw) > 0 {
					n := len(sk.raw)
					if n > 48 {
						n = 48
					}
					glog.Infof("xp update len=%d %s", len(sk.raw), hex.EncodeToString(sk.raw[:n]))
				}
				switch xpLogonHint(sk.raw) {
				case xpHintFail:
					c.Emit("error", fmt.Errorf("rdp logon failed: xp logon dialog"))
				case xpHintSuccess:
					c.Emit("logon", &SaveSessionInfo{InfoType: INFOTYPE_LOGON})
				}
			}
		}
	}
}

func (c *Client) RecvFastPath(secFlag byte, s []byte) {
	//glog.Debug("PDU RecvFastPath", hex.EncodeToString(s))
	glog.Debug("PDU RecvFastPath", secFlag&0x2 != 0)
	r := bytes.NewReader(s)
	for r.Len() > 0 {
		p, err := readFastPathUpdatePDU(r)
		if err != nil {
			glog.Debug("readFastPathUpdatePDU:", err)
			//continue
			return
		}
		if p.UpdateHeader == FASTPATH_UPDATETYPE_BITMAP {
			c.Emit("update", p.Data.(*FastPathBitmapUpdateDataPDU).Rectangles)
		}
	}
}

type InputEventsInterface interface {
	Serialize() []byte
}

func (c *Client) SendInputEvents(msgType uint16, events []InputEventsInterface) {
	pdu := &ClientInputEventPDU{}
	pdu.NumEvents = uint16(len(events))
	pdu.SlowPathInputEvents = make([]SlowPathInputEvent, 0, pdu.NumEvents)
	for _, in := range events {
		seria := in.Serialize()
		s := SlowPathInputEvent{0, msgType, len(seria), seria}
		pdu.SlowPathInputEvents = append(pdu.SlowPathInputEvents, s)
	}

	c.sendDataPDU(pdu)
}
