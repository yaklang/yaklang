package javaclassparser

const (
	StaticFlag    = 0x0008
	SyntheticFlag = 0x1000 // ACC_SYNTHETIC
)

// isSyntheticMethod reports whether the method carries the ACC_SYNTHETIC flag.
func isSyntheticMethod(accessFlags uint16) bool {
	return accessFlags&SyntheticFlag == SyntheticFlag
}
