package lib

import "fmt"

const (
	OPENVAS_ENCAPS_AUTO int = iota
	OPENVAS_ENCAPS_IP
	OPENVAS_ENCAPS_SSLv23
	OPENVAS_ENCAPS_SSLv2
	OPENVAS_ENCAPS_SSLv3
	OPENVAS_ENCAPS_TLSv1
	OPENVAS_ENCAPS_TLSv11
	OPENVAS_ENCAPS_TLSv12
	OPENVAS_ENCAPS_TLSv13
	OPENVAS_ENCAPS_TLScustom
	OPENVAS_ENCAPS_MAX
)

func GetEncapsName(code int) string {
	//code := OpenvasEncaps(c)
	switch code {
	case OPENVAS_ENCAPS_AUTO:
		return "auto"
	case OPENVAS_ENCAPS_IP:
		return "IP"
	case OPENVAS_ENCAPS_SSLv2:
		return "SSLv2"
	case OPENVAS_ENCAPS_SSLv23:
		return "SSLv23"
	case OPENVAS_ENCAPS_SSLv3:
		return "SSLv3"
	case OPENVAS_ENCAPS_TLSv1:
		return "TLSv1"
	case OPENVAS_ENCAPS_TLSv11:
		return "TLSv11"
	case OPENVAS_ENCAPS_TLSv12:
		return "TLSv12"
	case OPENVAS_ENCAPS_TLSv13:
		return "TLSv13"
	case OPENVAS_ENCAPS_TLScustom:
		return "TLScustom"
	default:
		return fmt.Sprintf("[unknown transport layer - code %d (0x%x)]",
			code, code)
	}
}
