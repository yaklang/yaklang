package ja3

var (
	Exports = map[string]interface{}{
		"ParseJA3":                      ParseJA3,
		"ParseJA3S":                     ParseJA3S,
		"ParseJA3ToClientHelloSpec":     ParseJA3ToClientHelloSpec,
		"GetTransportByClientHelloSpec": GetTransportByClientHelloSpec,
	}
)
