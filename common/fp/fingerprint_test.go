package fp

import (
	"testing"
	"time"
	"yaklang/common/utils"
)

func TestNewFingerprintMatcher(t *testing.T) {
	matcher, err := NewDefaultFingerprintMatcher(
		NewConfig(WithTransportProtos(UDP), WithProbesMax(3),
			WithProbeTimeoutHumanRead(2),
		))
	if err != nil {
		panic(err)
		return
	}

	for _, port := range utils.ParseStringToPorts("53,123,162,179,445,1194,1701,1812,5353") {
		port := port
		go func() {
			result, err := matcher.Match("ns1.cybertunnel.run", port)
			if err != nil {
				panic(err)
			}
			println(result.String())
		}()
	}

	time.Sleep(7 * time.Second)
}

func TestNewFingerprintMatcher1(t *testing.T) {
	matcher, err := NewDefaultFingerprintMatcher(
		NewConfig(WithTransportProtos(TCP), WithProbesMax(3),
			WithProbeTimeoutHumanRead(2), WithActiveMode(true),
		))
	if err != nil {
		panic(err)
		return
	}

	for _, port := range utils.ParseStringToPorts("8080") {
		port := port
		go func() {
			result, err := matcher.Match("127.0.0.1", port)
			if err != nil {
				panic(err)
			}
			println(result.String())
		}()
	}

	time.Sleep(7 * time.Second)
}
