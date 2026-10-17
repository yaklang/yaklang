package pcaputil

import "fmt"

// OPCUAObservationConfig selects finite wire observations. It does not establish
// authentication, namespace authority, custom schemas or endpoint success.
type OPCUAObservationConfig struct {
	Read, Browse, Write, BrowseNext, Call bool
	VariantArrays, StructuredValues       bool
}

// WithSessionOPCUAObservation exposes the same selected profiles as capture
// configuration while retaining the session's ParserBudget and event ownership.
func WithSessionOPCUAObservation(o OPCUAObservationConfig) ProtocolSessionOption {
	return func(s *captureSession) error {
		if (o.VariantArrays || o.StructuredValues) && !(o.Read || o.Browse || o.Write || o.BrowseNext || o.Call) {
			return fmt.Errorf("OPC UA value options require a selected service profile")
		}
		c := &s.f.a.config
		c.DecodeOPCUARead, c.DecodeOPCUABrowse, c.DecodeOPCUAWrite = o.Read, o.Browse, o.Write
		c.DecodeOPCUABrowseNext, c.DecodeOPCUACall = o.BrowseNext, o.Call
		c.DecodeOPCUAVariantArrays, c.DecodeOPCUAStructuredValues = o.VariantArrays, o.StructuredValues
		return nil
	}
}
