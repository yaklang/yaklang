package l2tpserver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
)

// handlePPPFrame processes a PPP frame from L2TP data message
func (s *Server) handlePPPFrame(session *Session, pppFrame []byte) {
	if len(pppFrame) < 2 {
		log.Errorf("PPP frame too short: %d bytes", len(pppFrame))
		return
	}

	// Check for PPP protocol field
	// PPP frames in L2TP may have Address/Control fields (0xFF 0x03) or just Protocol
	offset := 0
	if len(pppFrame) >= 2 && pppFrame[0] == 0xFF && pppFrame[1] == 0x03 {
		offset = 2 // Skip Address and Control fields
	}

	if len(pppFrame) < offset+2 {
		log.Errorf("PPP frame too short after skipping AC: %d bytes", len(pppFrame))
		return
	}

	// Check for IP packets (PPP protocol 0x0021 for IPv4) first
	protocol := binary.BigEndian.Uint16(pppFrame[offset : offset+2])
	if protocol == 0x0021 {
		// This is an IPv4 packet
		ipPacket := pppFrame[offset+2:]
		log.Infof("Received IP packet from session %d: %d bytes", session.sessionID, len(ipPacket))

		// Call packet callback if set
		if s.onPacket != nil {
			s.onPacket(ipPacket)
		}

		// Inject into network stack
		if s.endpoint != nil {
			err := s.InjectPacketToStack(ipPacket)
			if err != nil {
				log.Errorf("Inject packet to stack failed: %v", err)
			}
		}
		return // IP packet processed, done
	}

	// Parse PPP using bin-parser for control messages (LCP, IPCP, etc.)
	reader := bytes.NewReader(pppFrame)
	node, err := parser.ParseBinary(reader, "ppp", "PPP")
	if err != nil {
		log.Errorf("Parse PPP frame failed: %v", err)
		return
	}

	// Process PPP message
	resultParams, err := session.pppAuth.ProcessMessage(node)
	if err != nil {
		log.Errorf("Process PPP message failed: %v", err)
		return
	}

	// If we have a response to send
	if len(resultParams) > 0 {
		responseFrame, err := s.generatePPPResponse(resultParams)
		if err != nil {
			log.Errorf("Generate PPP response failed: %v", err)
			return
		}

		if responseFrame != nil {
			s.sendDataMessage(session, responseFrame)
		}
	}

	// Check authentication status
	select {
	case authOk := <-session.pppAuth.AuthOk:
		if authOk {
			log.Infof("Session %d authenticated successfully", session.sessionID)
			session.SetAuthenticated(true)

			// Allocate IP address
			sessionKey := fmt.Sprintf("%d-%d", session.tunnel.tunnelID, session.sessionID)
			clientIP, err := s.ipPool.Allocate(sessionKey)
			if err != nil {
				log.Errorf("Failed to allocate IP: %v", err)
				return
			}

			session.SetClientIP(clientIP)
			log.Infof("Allocated IP %s to session %d", clientIP, session.sessionID)

			// Send IPCP Configure-Request to assign IP
			s.sendIPCPConfigReq(session, clientIP)
		} else {
			log.Warnf("Session %d authentication failed", session.sessionID)
			session.SetAuthenticated(false)
		}
	default:
		// No auth result yet
	}
}

// generatePPPResponse generates PPP response frame
func (s *Server) generatePPPResponse(params map[string]any) ([]byte, error) {
	if len(params) == 0 {
		return nil, nil
	}

	// Generate PPP frame using bin-parser
	node, err := parser.GenerateBinary(params, "ppp", "PPP")
	if err != nil {
		return nil, err
	}

	return binparser.NodeToBytes(node), nil
}

// sendDataMessage sends a data message (PPP frame) to the client
func (s *Server) sendDataMessage(session *Session, pppFrame []byte) error {
	tunnel := session.tunnel

	// Create L2TP header for data message
	header := &L2TPHeader{
		Flags:     L2TPVersion, // Data message, no Type bit
		TunnelID:  tunnel.peerTunnelID,
		SessionID: session.peerSessionID,
	}

	// Serialize header
	headerData := header.Serialize()

	// Combine header and PPP frame
	packet := append(headerData, pppFrame...)

	// Send packet
	_, err := s.conn.WriteToUDP(packet, tunnel.remoteAddr)
	if err != nil {
		log.Errorf("Send data message failed: %v", err)
		return err
	}

	log.Debugf("Sent PPP frame to session %d: %d bytes", session.sessionID, len(pppFrame))
	return nil
}

// sendIPCPConfigReq sends IPCP Configure-Request to assign client IP
func (s *Server) sendIPCPConfigReq(session *Session, clientIP net.IP) error {
	// IPCP Configure-Request
	// Protocol: 0x8021 (IPCP)
	// Code: 1 (Configure-Request)

	serverIP := net.IPv4(172, 16, 0, 1)

	params := map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": uint16(0x8021), // IPCP
		"IPCP": map[string]any{
			"Code":       uint8(1), // Configure-Request
			"Identifier": uint8(1),
			"Length":     uint16(10),
			"Info": map[string]any{
				"Options": []map[string]any{
					{
						"Type":   uint8(3), // IP-Address
						"Length": uint8(6),
						"Data":   clientIP.To4(),
					},
				},
			},
		},
	}

	// Store server IP for this session
	session.mu.Lock()
	session.serverIP = serverIP
	session.mu.Unlock()

	pppFrame, err := s.generatePPPResponse(params)
	if err != nil {
		return err
	}

	return s.sendDataMessage(session, pppFrame)
}

// SendPPPFrame sends a PPP frame to a specific session (for testing or manual control)
func (s *Server) SendPPPFrame(tunnelID, sessionID uint16, pppFrame []byte) error {
	s.mu.RLock()
	tunnel, ok := s.tunnelsByID[tunnelID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("tunnel %d not found", tunnelID)
	}

	session, ok := tunnel.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("session %d not found in tunnel %d", sessionID, tunnelID)
	}

	return s.sendDataMessage(session, pppFrame)
}

// GetAuthFunc allows customizing authentication
func (s *Server) SetAuthFunc(authFunc func(username, password string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authFunc = authFunc
}

// processIPCPMessage processes IPCP messages
func (s *Server) processIPCPMessage(session *Session, node *base.Node) (map[string]any, error) {
	if node.Name != "IPCP" {
		return nil, fmt.Errorf("not IPCP message")
	}

	messageMap, ok := binparser.NodeToMap(node).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("convert IPCP message to map failed")
	}

	var ipcpCode uint8
	err := base.UnmarshalSubData(messageMap, "Code", &ipcpCode)
	if err != nil {
		return nil, fmt.Errorf("get IPCP code failed: %v", err)
	}

	switch ipcpCode {
	case 1: // Configure-Request
		// Client is requesting IP configuration
		// We should send Configure-Ack or Configure-Nak

		// For now, just acknowledge
		messageMap["Code"] = uint8(2) // Configure-Ack
		return messageMap, nil

	case 2: // Configure-Ack
		// Client acknowledged our configuration
		log.Infof("Session %d: IPCP Configure-Ack received", session.sessionID)

		// Signal that PPP is ready
		select {
		case session.pppReady <- struct{}{}:
		default:
		}

		return nil, nil

	case 3: // Configure-Nak
		// Client rejected our configuration
		log.Warnf("Session %d: IPCP Configure-Nak received", session.sessionID)
		return nil, nil

	case 4: // Configure-Reject
		// Client rejected our configuration
		log.Warnf("Session %d: IPCP Configure-Reject received", session.sessionID)
		return nil, nil
	}

	return nil, nil
}
