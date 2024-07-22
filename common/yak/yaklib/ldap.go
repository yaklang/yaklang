package yaklib

import (
	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

var LdapExports = map[string]interface{}{
	"Login": bruteutils.LdapLogin,

	"port":     bruteutils.Ldap_Port,
	"username": bruteutils.Ldap_Username,
	"password": bruteutils.Ldap_Password,
}
