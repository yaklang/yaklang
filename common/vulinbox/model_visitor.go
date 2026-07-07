package vulinbox

import (
	"time"

	"gorm.io/gorm"
)

type VulinVisitor struct {
	gorm.Model
	Username         string
	Password         string
	Age              int
	LastAccessDomain string
	LastAccessPath   string
	LastAccessTime   time.Time
	ProxyIp          string
	RealIp           string
}
