package yakgrpc

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestAppConfigDescriptorAdapter(t *testing.T) {
	type config struct {
		Token string `app:"id:1,name:token,desc:credential,required:true,type:string,default:old,verbose:Token,extra:secret"`
	}
	got, err := parseAppTagToOptions(&config{}, map[string]string{"token": "default:overridden"})
	require.NoError(t, err)
	require.Equal(t, []*ypb.ThirdPartyAppConfigItemTemplate{{Name: "token", Desc: "credential", Required: true, Type: "string", DefaultValue: "overridden", Verbose: "Token", Extra: "secret"}}, got)
	_, err = parseAppTagToOptions(nil)
	require.Error(t, err)
}
