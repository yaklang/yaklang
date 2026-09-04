package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func requireCSharpInterfaceDispatch(t *testing.T, code, sink, want string, rejects ...string) {
	t.Helper()
	prog := parseCSharpSemantics(t, code)
	flow, err := prog.SyntaxFlowWithError(sink + `(* #-> as $origin)`)
	require.NoError(t, err)
	origins := flow.GetValues("origin").String()
	require.Contains(t, origins, want)
	for _, reject := range rejects {
		require.NotContains(t, origins, reject)
	}
}

func TestCSharp_InterfaceDispatchUsesNearestImplementation(t *testing.T) {
	t.Run("new method reimplements interface", func(t *testing.T) {
		requireCSharpInterfaceDispatch(t, `
public interface INearestImplementation { object M(string value); }
public class NearestImplementationBase {
    public object M(string value) { return wrongInheritedSource(); }
}
public class NearestImplementationChild : NearestImplementationBase, INearestImplementation {
    public new object M(string value) { return nearestChildSource(); }
}
public class Program {
    public static void Read(INearestImplementation value) { nearestInterfaceSink(value.M("x")); }
    public static void Main() { Read(new NearestImplementationChild()); }
}
`, "nearestInterfaceSink", "nearestChildSource", "wrongInheritedSource")
	})

	t.Run("inherited implementation", func(t *testing.T) {
		requireCSharpInterfaceDispatch(t, `
public interface IInheritedImplementation { object M(string value); }
public class InheritedImplementationBase {
    public object M(string value) { return inheritedImplementationSource(); }
}
public class InheritedImplementationChild : InheritedImplementationBase, IInheritedImplementation { }
public class Program {
    public static void Read(IInheritedImplementation value) { inheritedInterfaceSink(value.M("x")); }
    public static void Main() { Read(new InheritedImplementationChild()); }
}
`, "inheritedInterfaceSink", "inheritedImplementationSource")
	})

	t.Run("direct implementation", func(t *testing.T) {
		requireCSharpInterfaceDispatch(t, `
public interface IDirectImplementation { object M(string value); }
public class DirectImplementation : IDirectImplementation {
    public object M(string value) { return directImplementationSource(); }
}
public class Program {
    public static void Read(IDirectImplementation value) { directInterfaceSink(value.M("x")); }
    public static void Main() { Read(new DirectImplementation()); }
}
`, "directInterfaceSink", "directImplementationSource")
	})

	t.Run("non-first overload", func(t *testing.T) {
		requireCSharpInterfaceDispatch(t, `
public interface IOverloadedImplementation {
    object M(int value);
    object M(string value);
}
public class OverloadedImplementation : IOverloadedImplementation {
    public object M(int value) { return wrongIntegerImplementation(); }
    public object M(string value) { return laterOverloadSource(); }
}
public class Program {
    public static void Read(IOverloadedImplementation value) { laterInterfaceSink(value.M("x")); }
    public static void Main() { Read(new OverloadedImplementation()); }
}
`, "laterInterfaceSink", "laterOverloadSource", "wrongIntegerImplementation")
	})
}
