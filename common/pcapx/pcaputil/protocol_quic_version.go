package pcaputil

const (
	quicVersion1 uint32 = 1
	quicVersion2 uint32 = 0x6b3343cf
)

func quicSupportedVersion(version uint32) bool {
	return version == quicVersion1 || version == quicVersion2
}

func quicDefaultVersion(version uint32) uint32 {
	if version == 0 {
		return quicVersion1
	}
	return version
}

func (q *binQUIC) effectiveVersion() uint32 {
	if q.version != 0 {
		return q.version
	}
	if q.keys != nil && q.keys.version != 0 {
		return q.keys.version
	}
	return quicVersion1
}

func quicNativeProfile(version uint32) string {
	switch version {
	case quicVersion1:
		return "quic-v1-native"
	case quicVersion2:
		return "quic-v2-native"
	default:
		return "quic-native"
	}
}
