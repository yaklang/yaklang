/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2023 WireGuard LLC. All Rights Reserved.
 */

package lowtun

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// socketDevice implements the Device interface using a socket connection.
type socketDevice struct {
	conn      net.Conn
	mtu       int
	utunName  string // actual TUN device name (e.g., "utun3")
	events    chan Event
	closeChan chan struct{}
	closeOnce sync.Once
	closed    bool
	mu        sync.RWMutex
}

// CreateDeviceFromSocket creates a Device from a socket connection.
// It dials the socket at the given path and returns a Device interface and the TUN device name.
// The client always reads the first server message which contains {"ok": true, "utun": "utunX"}.
// If secret is not empty, it will send authentication request first.
func CreateDeviceFromSocket(socketPath string, mtu int, secret string) (Device, string, error) {
	if mtu <= 0 {
		mtu = 1500 // Default MTU
	}

	conn, err := DialSocket(socketPath)
	if err != nil {
		return nil, "", err
	}

	// 始终读取第一个服务器消息（无论是否需要认证）
	var tunName string
	if secret != "" {
		// 需要认证：发送认证请求并读取响应
		log.Infof("authenticating with socket server...")
		var err error
		tunName, err = authenticateClient(conn, secret)
		if err != nil {
			conn.Close()
			return nil, "", utils.Errorf("authentication failed: %v", err)
		}
		log.Infof("authentication successful, utun: %s", tunName)
	} else {
		// 不需要认证：直接读取初始响应
		log.Infof("reading initial response from socket server...")
		var err error
		tunName, err = readInitialResponse(conn)
		if err != nil {
			conn.Close()
			return nil, "", utils.Errorf("failed to read initial response: %v", err)
		}
		log.Infof("received initial response, utun: %s", tunName)
	}

	dev := &socketDevice{
		conn:      conn,
		mtu:       mtu,
		utunName:  tunName,
		events:    make(chan Event, 10),
		closeChan: make(chan struct{}),
		closed:    false,
	}

	// Send initial Up event
	dev.events <- EventUp

	log.Infof("created socket device from %s with MTU %d, utun: %s", socketPath, mtu, tunName)

	return dev, tunName, nil
}

// authenticateClient 客户端认证：发送 {"secret": "..."} 并等待 {"ok": true, "utun": "..."}
// 返回 utun 名称
func authenticateClient(conn net.Conn, secret string) (string, error) {
	// 1. 发送认证请求
	authReq := map[string]string{"secret": secret}
	authReqData, err := json.Marshal(authReq)
	if err != nil {
		return "", utils.Errorf("failed to marshal auth request: %v", err)
	}

	// 写入长度前缀
	var lengthBuf [4]byte
	binary.BigEndian.PutUint32(lengthBuf[:], uint32(len(authReqData)))
	if _, err := conn.Write(lengthBuf[:]); err != nil {
		return "", utils.Errorf("failed to write auth request length: %v", err)
	}

	// 写入认证数据
	if _, err := conn.Write(authReqData); err != nil {
		return "", utils.Errorf("failed to write auth request: %v", err)
	}

	log.Debugf("sent auth request: %s", string(authReqData))

	// 2. 读取认证响应
	if _, err := io.ReadFull(conn, lengthBuf[:]); err != nil {
		return "", utils.Errorf("failed to read auth response length: %v", err)
	}

	respLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if respLen <= 0 || respLen > 1024 {
		return "", utils.Errorf("invalid auth response length: %d", respLen)
	}

	respData := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respData); err != nil {
		return "", utils.Errorf("failed to read auth response: %v", err)
	}

	log.Debugf("received auth response: %s", string(respData))

	// 3. 解析响应
	var authResp map[string]interface{}
	if err := json.Unmarshal(respData, &authResp); err != nil {
		return "", utils.Errorf("failed to unmarshal auth response: %v", err)
	}

	// 4. 检查认证结果
	if ok, exists := authResp["ok"]; !exists || ok != true {
		if errMsg, exists := authResp["error"]; exists {
			return "", utils.Errorf("authentication rejected: %v", errMsg)
		}
		return "", utils.Errorf("authentication rejected")
	}

	// 5. 提取 utun 名称
	tunName := ""
	if utun, exists := authResp["utun"]; exists {
		if tunNameStr, ok := utun.(string); ok {
			tunName = tunNameStr
		}
	}

	return tunName, nil
}

// readInitialResponse 读取服务器初始响应（不需要认证时）
func readInitialResponse(conn net.Conn) (string, error) {
	// 1. 读取响应长度
	var lengthBuf [4]byte
	if _, err := io.ReadFull(conn, lengthBuf[:]); err != nil {
		return "", utils.Errorf("failed to read initial response length: %v", err)
	}

	respLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if respLen <= 0 || respLen > 1024 {
		return "", utils.Errorf("invalid initial response length: %d", respLen)
	}

	// 2. 读取响应数据
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respData); err != nil {
		return "", utils.Errorf("failed to read initial response: %v", err)
	}

	log.Debugf("received initial response: %s", string(respData))

	// 3. 解析响应
	var resp map[string]interface{}
	if err := json.Unmarshal(respData, &resp); err != nil {
		return "", utils.Errorf("failed to unmarshal initial response: %v", err)
	}

	// 4. 检查响应
	if ok, exists := resp["ok"]; !exists || ok != true {
		if errMsg, exists := resp["error"]; exists {
			return "", utils.Errorf("server error: %v", errMsg)
		}
		return "", utils.Errorf("server returned ok=false")
	}

	// 5. 提取 utun 名称
	tunName := ""
	if utun, exists := resp["utun"]; exists {
		if tunNameStr, ok := utun.(string); ok {
			tunName = tunNameStr
		}
	}

	return tunName, nil
}

// Read reads packets from the socket connection.
// Each packet is prefixed with a 4-byte length header (network byte order).
// This implementation reads one packet at a time.
func (d *socketDevice) Read(bufs [][]byte, sizes []int, offset int) (n int, err error) {
	d.mu.RLock()
	if d.closed {
		d.mu.RUnlock()
		return 0, io.EOF
	}
	d.mu.RUnlock()

	// Validate input
	if len(bufs) == 0 || len(sizes) == 0 {
		return 0, utils.Errorf("empty buffers")
	}

	// Read 4-byte length header
	var lengthBuf [4]byte
	if _, err := io.ReadFull(d.conn, lengthBuf[:]); err != nil {
		return 0, err
	}

	packetLen := int(binary.BigEndian.Uint32(lengthBuf[:]))
	if packetLen <= 0 || packetLen > len(bufs[0])-offset {
		return 0, utils.Errorf("invalid packet length: %d", packetLen)
	}

	// Read packet data
	if _, err := io.ReadFull(d.conn, bufs[0][offset:offset+packetLen]); err != nil {
		return 0, err
	}

	sizes[0] = packetLen
	return 1, nil
}

// Write writes packets to the socket connection.
// Each packet is prefixed with a 4-byte length header (network byte order).
func (d *socketDevice) Write(bufs [][]byte, offset int) (int, error) {
	d.mu.RLock()
	if d.closed {
		d.mu.RUnlock()
		return 0, io.EOF
	}
	d.mu.RUnlock()

	n := 0
	for _, buf := range bufs {
		packetLen := len(buf) - offset
		if packetLen <= 0 {
			continue
		}

		// Write 4-byte length header
		var lengthBuf [4]byte
		binary.BigEndian.PutUint32(lengthBuf[:], uint32(packetLen))
		if _, err := d.conn.Write(lengthBuf[:]); err != nil {
			return n, err
		}

		// Write packet data
		if _, err := d.conn.Write(buf[offset:]); err != nil {
			return n, err
		}

		n++
	}

	return n, nil
}

// MTU returns the MTU of the device.
func (d *socketDevice) MTU() (int, error) {
	return d.mtu, nil
}

// Name returns the name of the actual TUN device (e.g., "utun3").
func (d *socketDevice) Name() (string, error) {
	return d.utunName, nil
}

// Events returns the event channel.
func (d *socketDevice) Events() <-chan Event {
	return d.events
}

// Close closes the socket device and releases resources.
func (d *socketDevice) Close() error {
	var err error
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		d.mu.Unlock()

		close(d.closeChan)
		close(d.events)

		if d.conn != nil {
			err = d.conn.Close()
		}

		log.Infof("socket device closed")
	})
	return err
}

// BatchSize returns the batch size for this device.
func (d *socketDevice) BatchSize() int {
	return 1 // Socket device processes one packet at a time
}
