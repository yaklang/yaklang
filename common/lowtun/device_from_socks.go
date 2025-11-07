/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2023 WireGuard LLC. All Rights Reserved.
 */

package lowtun

import (
	"encoding/binary"
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
	events    chan Event
	closeChan chan struct{}
	closeOnce sync.Once
	closed    bool
	mu        sync.RWMutex
}

// CreateDeviceFromSocket creates a Device from a socket connection.
// It dials the socket at the given path and returns a Device interface.
func CreateDeviceFromSocket(socketPath string, mtu int) (Device, error) {
	if mtu <= 0 {
		mtu = 1500 // Default MTU
	}

	conn, err := DialSocket(socketPath)
	if err != nil {
		return nil, err
	}

	dev := &socketDevice{
		conn:      conn,
		mtu:       mtu,
		events:    make(chan Event, 10),
		closeChan: make(chan struct{}),
		closed:    false,
	}

	// Send initial Up event
	dev.events <- EventUp

	log.Infof("created socket device from %s with MTU %d", socketPath, mtu)

	return dev, nil
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

// Name returns the name of the device (socket path).
func (d *socketDevice) Name() (string, error) {
	return "socket", nil
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
