package vulinbox

import (
	"time"

	"github.com/yaklang/gorm"
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
