package schema

import "gorm.io/gorm"

type TrafficTCPReassembledFrame struct {
	gorm.Model

	SessionUuid string `gorm:"index"`
	QuotedData  string
	Seq         int64
	Timestamp   int64
	Source      string
	Destination string
}
