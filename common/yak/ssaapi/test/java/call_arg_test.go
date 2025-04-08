package java

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestCallArg(t *testing.T) {
	code := `
import java.net.URI;
import java.net.URISyntaxException;

public class Main {
    public static void main(String[] args) throws URISyntaxException {
        URI uri = new URI("http://localhost:8080");
    }
}
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		vals, err := prog.SyntaxFlowWithError(`URI(* as $arg)`)
		require.NoError(t, err)
		arg := vals.GetValues("arg")
		require.Len(t, arg, 2)
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
