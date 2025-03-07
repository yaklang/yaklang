package match

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
)

type tlsProvider struct {
	PK gopacket.Packet

	// TLS层数据
	tls *layers.TLS

	// 缓存的证书数据
	certSubject     []byte
	certIssuer      []byte
	certSerial      []byte
	certFingerprint []byte
	sni             []byte
	random          []byte
	randomTime      []byte
	randomBytes     []byte
	version         string
	certChainLen    int

	// JA3相关数据
	ja3Hash    []byte
	ja3String  []byte
	ja3sHash   []byte
	ja3sString []byte
}

func newTLSProvider(pk gopacket.Packet) (*tlsProvider, error) {
	var tlsData layers.TLS
	tls := &tlsData
	var decoded []gopacket.LayerType
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeTLS, tls)
	err := parser.DecodeLayers(pk.ApplicationLayer().LayerContents(), &decoded)
	if err != nil {
		return nil, err
	}
	if tls == nil {
		return nil, fmt.Errorf("tls is nil")
	}

	provider := &tlsProvider{
		PK:  pk,
		tls: tls,
	}

	// 解析TLS数据
	provider.parseTLSData()
	return provider, nil
}

func (t *tlsProvider) parseTLSData() {
	if t.tls == nil {
		return
	}

	// 处理握手消息
	if len(t.tls.Handshake) > 0 {
		for _, hs := range t.tls.Handshake {
			// 解析握手数据
			t.parseHandshake(&hs)
		}
	}
}

func (t *tlsProvider) parseHandshake(hs *layers.TLSHandshakeRecord) {
	// 处理 ClientHello
	hello := hs.ClientHello
	if hello.HandshakeType == 1 { // ClientHello
		// 设置TLS版本
		version := uint16(hello.ProtocolVersion)
		switch version {
		case protocol.TLSVersion10:
			t.version = "1.0"
		case protocol.TLSVersion11:
			t.version = "1.1"
		case protocol.TLSVersion12:
			t.version = "1.2"
		case protocol.TLSVersion13:
			t.version = "1.3"
		}

		// 设置随机数
		if len(hello.Random) >= 32 {
			t.random = hello.Random
			t.randomTime = hello.Random[0:4]
			t.randomBytes = hello.Random[4:32]
		}

		// 设置 SNI
		if len(hello.SNI) > 0 {
			t.sni = hello.SNI
		}

		// 收集密码套件
		var ciphers []string
		for i := 0; i < len(hello.CipherSuits); i += 2 {
			if i+1 >= len(hello.CipherSuits) {
				break
			}
			cipher := uint16(hello.CipherSuits[i])<<8 | uint16(hello.CipherSuits[i+1])
			ciphers = append(ciphers, fmt.Sprintf("%d", cipher))
		}

		// 解析扩展
		var extensions []string
		var curves []string
		var pointFormats []string

		if len(hello.Extensions) >= 4 {
			extData := hello.Extensions
			for len(extData) >= 4 {
				extType := binary.BigEndian.Uint16(extData[0:2])
				extLen := binary.BigEndian.Uint16(extData[2:4])
				if len(extData) < 4+int(extLen) {
					break
				}

				// 记录扩展类型
				extensions = append(extensions, fmt.Sprintf("%d", extType))

				// 处理特殊扩展
				switch extType {
				case 0x000a: // 支持的曲线组
					curveData := extData[4 : 4+extLen]
					if len(curveData) >= 2 {
						curvesLen := binary.BigEndian.Uint16(curveData[0:2])
						curveData = curveData[2:]
						for i := 0; i < int(curvesLen); i += 2 {
							if len(curveData) < i+2 {
								break
							}
							curve := binary.BigEndian.Uint16(curveData[i:])
							curves = append(curves, fmt.Sprintf("%d", curve))
						}
					}
				case 0x000b: // EC点格式
					formatData := extData[4 : 4+extLen]
					if len(formatData) >= 1 {
						formatsLen := int(formatData[0])
						formatData = formatData[1:]
						for i := 0; i < formatsLen && i < len(formatData); i++ {
							format := formatData[i]
							pointFormats = append(pointFormats, fmt.Sprintf("%d", format))
						}
					}
				}

				extData = extData[4+extLen:]
			}
		}

		// 生成JA3字符串
		ja3Parts := []string{
			fmt.Sprintf("%d", version),
			strings.Join(ciphers, "-"),
			strings.Join(extensions, "-"),
			strings.Join(curves, "-"),
			strings.Join(pointFormats, "-"),
		}
		t.ja3String = []byte(strings.Join(ja3Parts, ","))

		// 生成JA3哈希
		hash := md5.Sum(t.ja3String)
		t.ja3Hash = []byte(fmt.Sprintf("%x", hash))
	}
}

func (t *tlsProvider) Get(mdf modifier.Modifier) []byte {
	switch mdf {
	case modifier.TLSCertSubject:
		return t.certSubject
	case modifier.TLSCertIssuer:
		return t.certIssuer
	case modifier.TLSCertSerial:
		return t.certSerial
	case modifier.TLSCertFingerprint:
		return t.certFingerprint
	case modifier.TLSSNI:
		return t.sni
	case modifier.TLSRandom:
		return t.random
	case modifier.TLSRandomTime:
		return t.randomTime
	case modifier.TLSRandomBytes:
		return t.randomBytes
	case modifier.JA3Hash:
		return t.ja3Hash
	case modifier.JA3String:
		return t.ja3String
	case modifier.JA3SHash:
		return t.ja3sHash
	case modifier.JA3SString:
		return t.ja3sString
	case modifier.Default:
		return t.tls.LayerPayload()
	}
	return nil
}

func tlsParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return fmt.Errorf("content rule config is nil")
	}

	// 创建TLS提供者
	provider, err := newTLSProvider(c.PK)
	if err != nil {
		return err
	}
	if !c.Must(provider != nil) {
		return fmt.Errorf("failed to create TLS provider")
	}

	// 注册缓冲区提供者
	c.SetBufferProvider(provider.Get)
	c.Value["tls"] = provider

	return nil
}

func tlsMatcher(c *matchContext) error {
	if c.Rule.ContentRuleConfig.TLSConfig == nil {
		return fmt.Errorf("TLS config is nil")
	}

	provider, ok := c.Value["tls"].(*tlsProvider)
	if !ok || provider == nil {
		return fmt.Errorf("TLS provider not found or invalid")
	}

	cfg := c.Rule.ContentRuleConfig.TLSConfig

	// 匹配TLS版本
	if cfg.Version != "" {
		if provider.version == "" {
			return fmt.Errorf("TLS version not found")
		}
		if !c.Must(cfg.Version == provider.version) {
			return fmt.Errorf("TLS version mismatch: expected %s, got %s", cfg.Version, provider.version)
		}
	}

	// 匹配证书链长度
	if cfg.CertChainLen != nil {
		if !c.Must(cfg.CertChainLen.Match(provider.certChainLen)) {
			return fmt.Errorf("certificate chain length mismatch: expected %v, got %d", cfg.CertChainLen, provider.certChainLen)
		}
	}

	// 匹配JA3哈希
	if cfg.JA3Hash != "" {
		if len(provider.ja3Hash) == 0 {
			return fmt.Errorf("JA3 hash not found")
		}
		if !c.Must(string(provider.ja3Hash) == cfg.JA3Hash) {
			return fmt.Errorf("JA3 hash mismatch: expected %s, got %s", cfg.JA3Hash, string(provider.ja3Hash))
		}
	}

	// 匹配JA3字符串
	if cfg.JA3String != "" {
		if len(provider.ja3String) == 0 {
			return fmt.Errorf("JA3 string not found")
		}
		if !c.Must(string(provider.ja3String) == cfg.JA3String) {
			return fmt.Errorf("JA3 string mismatch: expected %s, got %s", cfg.JA3String, string(provider.ja3String))
		}
	}

	// 匹配JA3S哈希
	if cfg.JA3SHash != "" {
		if len(provider.ja3sHash) == 0 {
			return fmt.Errorf("JA3S hash not found")
		}
		if !c.Must(string(provider.ja3sHash) == cfg.JA3SHash) {
			return fmt.Errorf("JA3S hash mismatch: expected %s, got %s", cfg.JA3SHash, string(provider.ja3sHash))
		}
	}

	// 匹配JA3S字符串
	if cfg.JA3SString != "" {
		if len(provider.ja3sString) == 0 {
			return fmt.Errorf("JA3S string not found")
		}
		if !c.Must(string(provider.ja3sString) == cfg.JA3SString) {
			return fmt.Errorf("JA3S string mismatch: expected %s, got %s", cfg.JA3SString, string(provider.ja3sString))
		}
	}

	// 匹配证书是否过期
	if cfg.CertExpired != nil {
		// TODO: 由于目前无法解析证书，暂时跳过证书过期检查
		// 需要等实现了证书解析后再添加这部分功能
		return fmt.Errorf("certificate expiration check not implemented")
	}

	return nil
}
