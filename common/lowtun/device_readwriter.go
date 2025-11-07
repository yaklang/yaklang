/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2023 WireGuard LLC. All Rights Reserved.
 */

package lowtun

import (
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// deviceReadWriter wraps a Device to implement io.ReadWriter interface.
type deviceReadWriter struct {
	device Device
	mtu    int
	offset int

	// Read buffer
	readBuf []byte

	readMu  sync.Mutex
	writeMu sync.Mutex
}

// ConvertTUNDeviceToReadWriter converts a Device to an io.ReadWriter.
// This allows using standard io.Copy for bidirectional forwarding.
// offset specifies where the device expects to read/write data (e.g., 4 for TUN devices with headers).
func ConvertTUNDeviceToReadWriter(device Device, offset int) (io.ReadWriter, error) {
	mtu, err := device.MTU()
	if err != nil {
		return nil, utils.Errorf("failed to get device MTU: %v", err)
	}

	return &deviceReadWriter{
		device:  device,
		mtu:     mtu,
		offset:  offset,
		readBuf: make([]byte, mtu+offset),
	}, nil
}

// Read implements io.Reader interface.
// It reads one packet from the device and returns the packet data (without headers).
// This method is thread-safe.
func (rw *deviceReadWriter) Read(p []byte) (n int, err error) {
	rw.readMu.Lock()
	defer rw.readMu.Unlock()

	bufs := [][]byte{rw.readBuf}
	sizes := []int{0}

	// Read one packet from device
	nPackets, err := rw.device.Read(bufs, sizes, rw.offset)
	if err != nil {
		return 0, err
	}

	if nPackets == 0 {
		return 0, io.EOF
	}

	// Copy packet data to output buffer
	packetSize := sizes[0]
	if packetSize > len(p) {
		return 0, utils.Errorf("buffer too small: need %d, have %d", packetSize, len(p))
	}

	copy(p, rw.readBuf[rw.offset:rw.offset+packetSize])
	return packetSize, nil
}

// Write implements io.Writer interface.
// It writes one packet to the device.
// This method is thread-safe.
func (rw *deviceReadWriter) Write(p []byte) (n int, err error) {
	rw.writeMu.Lock()
	defer rw.writeMu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	if len(p) > rw.mtu {
		return 0, utils.Errorf("packet too large: %d > %d", len(p), rw.mtu)
	}

	// Prepare buffer with offset
	buf := make([]byte, len(p)+rw.offset)
	copy(buf[rw.offset:], p)

	// Write to device
	nPackets, err := rw.device.Write([][]byte{buf}, rw.offset)
	if err != nil {
		return 0, err
	}

	if nPackets == 0 {
		return 0, utils.Errorf("failed to write packet")
	}

	return len(p), nil
}
