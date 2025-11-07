//go:build windows
// +build windows

/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2023 WireGuard LLC. All Rights Reserved.
 */

package lowtun

import (
	"net"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/natefinch/npipe.v2"
)

// ListenSocket creates a named pipe listener on Windows at the specified path.
// The path will be converted to Windows named pipe format (\\.\pipe\name).
func ListenSocket(socketPath string) (net.Listener, error) {
	// Convert path to Windows named pipe format
	pipeName := toNamedPipePath(socketPath)

	// Create named pipe listener
	listener, err := npipe.Listen(pipeName)
	if err != nil {
		return nil, utils.Errorf("failed to listen on named pipe %s: %v", pipeName, err)
	}

	return listener, nil
}

// DialSocket connects to a named pipe on Windows at the specified path.
func DialSocket(socketPath string) (net.Conn, error) {
	// Convert path to Windows named pipe format
	pipeName := toNamedPipePath(socketPath)

	// Connect to named pipe
	conn, err := npipe.Dial(pipeName)
	if err != nil {
		return nil, utils.Errorf("failed to dial named pipe %s: %v", pipeName, err)
	}
	return conn, nil
}

// toNamedPipePath converts a socket path to Windows named pipe format.
// For example: "/tmp/test.sock" -> "\\.\pipe\test"
func toNamedPipePath(socketPath string) string {
	// If already in named pipe format, return as is
	if strings.HasPrefix(socketPath, `\\.\pipe\`) || strings.HasPrefix(socketPath, `//./pipe/`) {
		return socketPath
	}

	// Extract base name from path
	name := socketPath
	if idx := strings.LastIndexAny(socketPath, `/\`); idx >= 0 {
		name = socketPath[idx+1:]
	}

	// Remove extension if present
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[:idx]
	}

	return `\\.\pipe\` + name
}
