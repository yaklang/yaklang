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

	// For PAP, try direct parsing first
	if protocol == 0xc023 { // PAP
		// Extract PAP data (skip Address, Control, and Protocol fields)
		papData := pppFrame[offset+2:]
		s.handlePAPDirect(session, papData)
		return
	}

	// Parse PPP using bin-parser for other control messages (LCP, IPCP, etc.)
	reader := bytes.NewReader(pppFrame)
	node, err := parser.ParseBinary(reader, "ppp", "PPP")
	if err != nil {
		log.Errorf("Parse PPP frame failed: %v", err)
		return
	}

	// Check if this is an IPCP message
	if protocol == 0x8021 { // IPCP
		ipcpNode := base.GetNodeByPath(node, "Information.IPCP")
		if ipcpNode == nil {
			ipcpNode = base.GetNodeByPath(node, "@PPP.IPCP")
		}
		if ipcpNode != nil {
			resultParams, err := s.processIPCPMessage(session, ipcpNode)
			if err != nil {
				log.Errorf("Process IPCP message failed: %v", err)
				return
			}

			if len(resultParams) > 0 {
				// Wrap IPCP response in PPP frame
				pppParams := map[string]any{
					"Address":  0xff,
					"Control":  0x03,
					"Protocol": uint16(0x8021),
					"IPCP":     resultParams,
				}
				responseFrame, err := s.generatePPPResponse(pppParams)
				if err != nil {
					log.Errorf("Generate IPCP response failed: %v", err)
					return
				}
				if responseFrame != nil {
					s.sendDataMessage(session, responseFrame)
				}
			}
			return
		}
	}

	// Server-side PPP authentication handling
	s.handlePPPAuth(session, node, protocol)
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

// SetAuthFunc allows customizing authentication
func (s *Server) SetAuthFunc(authFunc func(username, password string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authFunc = authFunc
}

// handlePPPAuth handles PPP authentication messages (LCP, PAP, CHAP)
func (s *Server) handlePPPAuth(session *Session, node *base.Node, protocol uint16) {
	switch protocol {
	case 0xc021: // LCP
		s.handleLCPMessage(session, node)
	case 0xc023: // PAP
		s.handlePAPMessage(session, node)
	case 0xc223: // CHAP
		s.handleCHAPMessage(session, node)
	default:
		log.Debugf("Unhandled PPP protocol: 0x%04x", protocol)
	}
}

// handlePAPDirect handles PAP authentication by directly parsing the raw data
func (s *Server) handlePAPDirect(session *Session, papData []byte) {
	if len(papData) < 4 {
		log.Errorf("PAP data too short: %d bytes", len(papData))
		return
	}

	papCode := papData[0]
	papID := papData[1]
	papLength := binary.BigEndian.Uint16(papData[2:4])

	log.Debugf("PAP: Code=%d, ID=%d, Length=%d", papCode, papID, papLength)

	// PAP Request (Code 1)
	if papCode != 1 {
		log.Debugf("Received PAP code %d (not a request)", papCode)
		return
	}

	if len(papData) < int(papLength) {
		log.Errorf("PAP data incomplete: have %d bytes, need %d bytes", len(papData), papLength)
		return
	}

	// Parse PAP request format: Code(1) + ID(1) + Length(2) + UsernameLen(1) + Username + PasswordLen(1) + Password
	offset := 4
	if offset >= int(papLength) {
		log.Errorf("PAP data has no username length field")
		return
	}

	usernameLen := int(papData[offset])
	offset++

	if offset+usernameLen > int(papLength) {
		log.Errorf("PAP username exceeds data length")
		return
	}

	username := string(papData[offset : offset+usernameLen])
	offset += usernameLen

	if offset >= int(papLength) {
		log.Errorf("PAP data has no password length field")
		return
	}

	passwordLen := int(papData[offset])
	offset++

	if offset+passwordLen > int(papLength) {
		log.Errorf("PAP password exceeds data length")
		return
	}

	password := string(papData[offset : offset+passwordLen])

	log.Infof("PAP authentication request: username=%s", username)

	// Authenticate
	authOk := false
	s.mu.RLock()
	authFunc := s.authFunc
	s.mu.RUnlock()

	if authFunc != nil {
		authOk = authFunc(username, password)
	} else {
		// If no auth function, accept all
		authOk = true
	}

	// Generate response
	var responseCode uint8
	var message string
	if authOk {
		responseCode = 2 // PAP-Ack
		message = "Authentication successful"
		log.Infof("PAP authentication successful for user: %s", username)
		session.SetAuthenticated(true)

		// Allocate IP address
		sessionKey := fmt.Sprintf("%d-%d", session.tunnel.tunnelID, session.sessionID)
		clientIP, err := s.ipPool.Allocate(sessionKey)
		if err != nil {
			log.Errorf("Failed to allocate IP: %v", err)
			responseCode = 3 // PAP-Nak
			message = "IP allocation failed"
			authOk = false
		} else {
			session.SetClientIP(clientIP)
			log.Infof("Allocated IP %s to session %d", clientIP, session.sessionID)
		}
	} else {
		responseCode = 3 // PAP-Nak
		message = "Authentication failed"
		log.Warnf("PAP authentication failed for user: %s", username)
		session.SetAuthenticated(false)
	}

	// Build PAP response manually to avoid bin-parser issues
	messageBytes := []byte(message)
	papResponseLength := uint16(5 + len(messageBytes))

	papResponse := make([]byte, papResponseLength)
	papResponse[0] = responseCode                                   // Code: PAP-Ack (2) or PAP-Nak (3)
	papResponse[1] = papID                                          // Identifier
	binary.BigEndian.PutUint16(papResponse[2:4], papResponseLength) // Length
	papResponse[4] = uint8(len(messageBytes))                       // Message Length
	copy(papResponse[5:], messageBytes)                             // Message

	// Wrap in PPP frame
	pppFrame := make([]byte, 4+len(papResponse))
	pppFrame[0] = 0xFF                                // Address
	pppFrame[1] = 0x03                                // Control
	binary.BigEndian.PutUint16(pppFrame[2:4], 0xC023) // Protocol: PAP
	copy(pppFrame[4:], papResponse)

	s.sendDataMessage(session, pppFrame)

	// If authentication was successful, send IPCP Configure-Request
	if authOk {
		clientIP := session.GetClientIP()
		s.sendIPCPConfigReq(session, clientIP)
	}
}

// handleLCPMessage handles LCP messages
func (s *Server) handleLCPMessage(session *Session, node *base.Node) {
	lcpNode := base.GetNodeByPath(node, "Information.LCP")
	if lcpNode == nil {
		lcpNode = base.GetNodeByPath(node, "@PPP.LCP")
	}
	if lcpNode == nil {
		log.Errorf("Cannot find LCP node")
		return
	}

	messageMap, ok := binparser.NodeToMap(lcpNode).(map[string]any)
	if !ok {
		log.Errorf("Convert LCP message to map failed")
		return
	}

	var lcpCode, lcpID uint8
	err := base.UnmarshalSubData(messageMap, "Code", &lcpCode)
	if err != nil {
		log.Errorf("Get LCP code failed: %v", err)
		return
	}

	err = base.UnmarshalSubData(messageMap, "Identifier", &lcpID)
	if err != nil {
		log.Errorf("Get LCP identifier failed: %v", err)
		return
	}

	log.Debugf("Received LCP Code=%d, ID=%d", lcpCode, lcpID)

	switch lcpCode {
	case 1: // Configure-Request
		// Acknowledge the request
		messageMap["Code"] = uint8(2) // Configure-Ack
		pppParams := map[string]any{
			"Address":  0xff,
			"Control":  0x03,
			"Protocol": uint16(0xc021),
			"LCP":      messageMap,
		}

		responseFrame, err := s.generatePPPResponse(pppParams)
		if err != nil {
			log.Errorf("Generate LCP response failed: %v", err)
			return
		}

		if responseFrame != nil {
			s.sendDataMessage(session, responseFrame)
		}

	case 2: // Configure-Ack
		log.Infof("Session %d: LCP Configure-Ack received", session.sessionID)

	case 3: // Configure-Nak
		log.Warnf("Session %d: LCP Configure-Nak received", session.sessionID)

	case 4: // Configure-Reject
		log.Warnf("Session %d: LCP Configure-Reject received", session.sessionID)
	}
}

// handlePAPMessage handles PAP authentication messages
func (s *Server) handlePAPMessage(session *Session, node *base.Node) {
	papNode := base.GetNodeByPath(node, "@PPP.PAP")

	var papCode, papID uint8
	var username, password string

	// Try bin-parser first
	if papNode != nil {
		messageMap, ok := binparser.NodeToMap(papNode).(map[string]any)
		if ok {
			// Try to extract using bin-parser
			err := base.UnmarshalSubData(messageMap, "Code", &papCode)
			if err == nil {
				base.UnmarshalSubData(messageMap, "Identifier", &papID)

				// Try to extract username
				var usernameLen uint8
				var usernameBytes []byte
				err = base.UnmarshalSubData(messageMap, "Data.UsernameLength", &usernameLen)
				if err == nil {
					base.UnmarshalSubData(messageMap, "Data.Username", &usernameBytes)
					if len(usernameBytes) > 0 {
						if int(usernameLen) <= len(usernameBytes) {
							username = string(usernameBytes[:usernameLen])
						} else {
							username = string(usernameBytes)
						}
					}
				}

				// Try to extract password
				var passwordLen uint8
				var passwordBytes []byte
				err = base.UnmarshalSubData(messageMap, "Data.PasswordLength", &passwordLen)
				if err == nil {
					base.UnmarshalSubData(messageMap, "Data.Password", &passwordBytes)
					if len(passwordBytes) > 0 {
						if int(passwordLen) <= len(passwordBytes) {
							password = string(passwordBytes[:passwordLen])
						} else {
							password = string(passwordBytes)
						}
					}
				}
			}
		}
	}

	// If bin-parser failed, we should have gotten the raw PPP frame data somewhere
	// For now, log that parsing failed
	if username == "" && password == "" {
		log.Errorf("Cannot find PAP node or parse PAP data")
		return
	}

	// PAP Request (Code 1)
	if papCode != 1 {
		log.Debugf("Received PAP code %d (not a request)", papCode)
		return // Not a request
	}

	log.Infof("PAP authentication request: username=%s", username)

	// Authenticate
	authOk := false
	s.mu.RLock()
	authFunc := s.authFunc
	s.mu.RUnlock()

	if authFunc != nil {
		authOk = authFunc(username, password)
	} else {
		// If no auth function, accept all
		authOk = true
	}

	// Generate response
	var responseCode uint8
	var message string
	if authOk {
		responseCode = 2 // PAP-Ack
		message = "Authentication successful"
		log.Infof("PAP authentication successful for user: %s", username)
		session.SetAuthenticated(true)

		// Allocate IP address
		sessionKey := fmt.Sprintf("%d-%d", session.tunnel.tunnelID, session.sessionID)
		clientIP, err := s.ipPool.Allocate(sessionKey)
		if err != nil {
			log.Errorf("Failed to allocate IP: %v", err)
			responseCode = 3 // PAP-Nak
			message = "IP allocation failed"
			authOk = false
		} else {
			session.SetClientIP(clientIP)
			log.Infof("Allocated IP %s to session %d", clientIP, session.sessionID)
		}
	} else {
		responseCode = 3 // PAP-Nak
		message = "Authentication failed"
		log.Warnf("PAP authentication failed for user: %s", username)
		session.SetAuthenticated(false)
	}

	messageBytes := []byte(message)
	response := map[string]any{
		"Code":       responseCode,
		"Identifier": papID,
		"Length":     uint16(5 + len(messageBytes)),
		"Data": map[string]any{
			"MessageLength": uint8(len(messageBytes)),
			"Message":       messageBytes,
		},
	}

	pppParams := map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": uint16(0xc023),
		"PAP":      response,
	}

	responseFrame, err := s.generatePPPResponse(pppParams)
	if err != nil {
		log.Errorf("Generate PAP response failed: %v", err)
		return
	}

	if responseFrame != nil {
		s.sendDataMessage(session, responseFrame)
	}

	// If authentication was successful, send IPCP Configure-Request
	if authOk {
		clientIP := session.GetClientIP()
		s.sendIPCPConfigReq(session, clientIP)
	}
}

// handleCHAPMessage handles CHAP authentication messages
func (s *Server) handleCHAPMessage(session *Session, node *base.Node) {
	chapNode := base.GetNodeByPath(node, "Information.CHAP")
	if chapNode == nil {
		chapNode = base.GetNodeByPath(node, "@PPP.CHAP")
	}
	if chapNode == nil {
		log.Errorf("Cannot find CHAP node")
		return
	}

	messageMap, ok := binparser.NodeToMap(chapNode).(map[string]any)
	if !ok {
		log.Errorf("Convert CHAP message to map failed")
		return
	}

	var chapCode, chapID uint8
	err := base.UnmarshalSubData(messageMap, "Code", &chapCode)
	if err != nil {
		log.Errorf("Get CHAP code failed: %v", err)
		return
	}

	err = base.UnmarshalSubData(messageMap, "Identifier", &chapID)
	if err != nil {
		log.Errorf("Get CHAP identifier failed: %v", err)
		return
	}

	log.Debugf("Received CHAP Code=%d, ID=%d", chapCode, chapID)

	// CHAP Response (Code 2)
	if chapCode == 2 {
		// This is a CHAP response from the client
		// For now, we'll accept it
		// A full implementation would validate the response
		log.Infof("Session %d: CHAP response received, accepting", session.sessionID)
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

		// Send CHAP Success
		message := []byte("Authentication successful")
		response := map[string]any{
			"Code":       uint8(3), // CHAP Success
			"Identifier": chapID,
			"Length":     uint16(4 + len(message)),
			"Data": map[string]any{
				"Message": message,
			},
		}

		pppParams := map[string]any{
			"Address":  0xff,
			"Control":  0x03,
			"Protocol": uint16(0xc223),
			"CHAP":     response,
		}

		responseFrame, err := s.generatePPPResponse(pppParams)
		if err != nil {
			log.Errorf("Generate CHAP response failed: %v", err)
			return
		}

		if responseFrame != nil {
			s.sendDataMessage(session, responseFrame)
		}

		// Send IPCP Configure-Request
		s.sendIPCPConfigReq(session, clientIP)
	}
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
