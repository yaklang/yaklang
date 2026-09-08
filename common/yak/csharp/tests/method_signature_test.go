package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_Expression_AliasAndCLRNamesShareVirtualSignature(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class SignatureBase {
    public virtual object M(string value) { return baseSignatureSource(); }
}
public class SignatureChild : SignatureBase {
    public override object M(global::System.String value) { return childSignatureSource(); }
}
public class SignatureHarness {
    public static object Run(SignatureBase receiver) {
        return finalSink(receiver.M("value"));
    }
}
public class Program {
    public static void Main() { SignatureHarness.Run(new SignatureChild()); }
}
`)

	flow, err := prog.SyntaxFlowWithError(`finalSink(* #-> as $origin)`)
	require.NoError(t, err)
	require.Contains(t, flow.GetValues("origin").String(), "childSignatureSource")
}
