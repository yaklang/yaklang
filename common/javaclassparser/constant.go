package javaclassparser

const (
	StaticFlag    = 0x0008
	SyntheticFlag = 0x1000 // ACC_SYNTHETIC
	BridgeFlag    = 0x0040 // ACC_BRIDGE (methods only; same bit as ACC_VOLATILE for fields)
)

// isSyntheticMethod reports whether the method carries the ACC_SYNTHETIC flag.
func isSyntheticMethod(accessFlags uint16) bool {
	return accessFlags&SyntheticFlag == SyntheticFlag
}

// isBridgeMethod reports whether the method is a compiler-generated bridge method
// (ACC_BRIDGE). javac synthesizes these to implement covariant returns and generic
// erasure (e.g. a class implementing Builder<String> gets a synthetic `Object build()`
// that delegates to `String build()`). They are not source-level methods: dumping them
// produces illegal Java source (two methods differing only by return type), so they
// must be suppressed during decompilation. CFR/Vineflower suppress them too.
func isBridgeMethod(accessFlags uint16) bool {
	return accessFlags&BridgeFlag == BridgeFlag
}
