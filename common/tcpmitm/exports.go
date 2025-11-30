package tcpmitm

// This file provides convenience functions and type aliases for the tcpmitm package.

// FrameDirection type aliases for easy use
const (
	// ClientToServer indicates data flowing from client to server.
	ClientToServer = DirectionClientToServer

	// ServerToClient indicates data flowing from server to client.
	ServerToClient = DirectionServerToClient
)

// Split strategy aliases
const (
	// TimeGap splits based on silence intervals.
	TimeGap = SplitByTimeGap

	// Direction splits when data direction changes.
	Direction = SplitByDirection

	// FixedSize splits when buffer reaches a fixed size.
	FixedSize = SplitBySize

	// NoSplit performs transparent forwarding.
	NoSplit = SplitNone
)
