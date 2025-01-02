package lowtun

type DriverProvider int
type DriverType int

const (
	// Driver_MacOSDriverSystem refers to the default P2P driver
	Driver_MacOSDriverSystem DriverProvider = 0
	// MacOSDriverTunTapOSX refers to the third-party tuntaposx driver
	// see https://sourceforge.net/p/tuntaposx
	Driver_MacOSDriverTunTapOSX DriverProvider = 1
)

const (
	_ DriverType = iota
	TUN
	TAP
)
