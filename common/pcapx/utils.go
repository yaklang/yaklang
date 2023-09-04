package pcapx

import (
	"github.com/google/gopacket"
	"github.com/pkg/errors"
)

func seriGopkt(layers ...gopacket.SerializableLayer) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, layers...)
	if err != nil {
		return nil, errors.Wrap(err, "serialize gopacket error")
	}
	return buffer.Bytes(), nil
}
