package core

// Target describes an image or code region independently of its file format.
// Bits is the address width, not the width of every instruction operand.
// ABI and Machine retain format-provided identifiers without guessing an OS.
// Mode carries ISA-specific execution-mode information (for example Thumb).
// A parser must reject modes or target combinations it cannot interpret.
type Target struct {
	OS           OperatingSystem `json:"os"`
	Architecture Architecture    `json:"architecture"`
	Bits         int             `json:"bits"`
	ByteOrder    ByteOrder       `json:"byte_order"`
	ABI          string          `json:"abi,omitempty"`
	Machine      string          `json:"machine,omitempty"`
	Mode         string          `json:"mode,omitempty"`
}
