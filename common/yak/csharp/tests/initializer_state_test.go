package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_InstanceInitializerStateAndConstructorOverride(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class FieldBox {
    public string Value = fieldInitSource();
    public FieldBox() { Value = fieldCtorSource(); }
}

public class DefaultFieldBox {
    public string Value = defaultFieldInitSource();
}

public class ExplicitUntouchedFieldBox {
    public string Value = untouchedFieldInitSource();
    public ExplicitUntouchedFieldBox() { }
}

public class PropertyBox {
    public string Value { get; set; } = propertyInitSource();
    public PropertyBox() { Value = propertyCtorSource(); }
}

public class DefaultPropertyBox {
    public string Value { get; set; } = defaultPropertyInitSource();
}

public class ExplicitUntouchedPropertyBox {
    public string Value { get; set; } = untouchedPropertyInitSource();
    public ExplicitUntouchedPropertyBox() { }
}

public class PostConstructionBox {
    public string Value = postInitSource();
    public PostConstructionBox() { Value = postCtorSource(); }
}

public class BaseStateBox {
    public string Value = baseInitSource();
    public BaseStateBox() { Value = baseCtorSource(); }
}

public class GeneratedChildStateBox : BaseStateBox { }

public class Program {
    public static void Main(string[] args) {
        fieldOverrideSink(new FieldBox().Value);
        fieldDefaultSink(new DefaultFieldBox().Value);
        fieldUntouchedSink(new ExplicitUntouchedFieldBox().Value);
        propertyOverrideSink(new PropertyBox().Value);
        propertyDefaultSink(new DefaultPropertyBox().Value);
        propertyUntouchedSink(new ExplicitUntouchedPropertyBox().Value);

        var post = new PostConstructionBox();
        post.Value = postAssignSource();
        postAssignSink(post.Value);
        objectInitializerSink(new PostConstructionBox { Value = objectInitializerSource() }.Value);
        inheritedOverrideSink(new GeneratedChildStateBox().Value);
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`
fieldOverrideSink(* #-> as $fieldOverride);
fieldDefaultSink(* #-> as $fieldDefault);
fieldUntouchedSink(* #-> as $fieldUntouched);
propertyOverrideSink(* #-> as $propertyOverride);
propertyDefaultSink(* #-> as $propertyDefault);
propertyUntouchedSink(* #-> as $propertyUntouched);
postAssignSink(* #-> as $postAssign);
objectInitializerSink(* #-> as $objectInitializer);
inheritedOverrideSink(* #-> as $inheritedOverride)
`)
	require.NoError(t, err)

	fieldOverride := flow.GetValues("fieldOverride").String()
	require.Contains(t, fieldOverride, "fieldCtorSource")
	require.NotContains(t, fieldOverride, "fieldInitSource", "constructor assignment must overwrite the field initializer")
	require.Contains(t, flow.GetValues("fieldDefault").String(), "defaultFieldInitSource", "unmodified field initializer must remain visible")
	require.Contains(t, flow.GetValues("fieldUntouched").String(), "untouchedFieldInitSource", "an explicit constructor that does not write the field must preserve its initializer")

	propertyOverride := flow.GetValues("propertyOverride").String()
	require.Contains(t, propertyOverride, "propertyCtorSource")
	require.NotContains(t, propertyOverride, "propertyInitSource", "constructor assignment must overwrite the auto-property initializer")
	require.Contains(t, flow.GetValues("propertyDefault").String(), "defaultPropertyInitSource", "unmodified auto-property initializer must remain visible")
	require.Contains(t, flow.GetValues("propertyUntouched").String(), "untouchedPropertyInitSource", "an explicit constructor that does not write the auto-property must preserve its initializer")

	postAssign := flow.GetValues("postAssign").String()
	require.Contains(t, postAssign, "postAssignSource")
	require.NotContains(t, postAssign, "postCtorSource", "a post-construction assignment must overwrite constructor state")
	require.NotContains(t, postAssign, "postInitSource")

	objectInitializer := flow.GetValues("objectInitializer").String()
	require.Contains(t, objectInitializer, "objectInitializerSource")
	require.NotContains(t, objectInitializer, "postCtorSource", "an object initializer must overwrite constructor state")
	require.NotContains(t, objectInitializer, "postInitSource")

	inheritedOverride := flow.GetValues("inheritedOverride").String()
	require.Contains(t, inheritedOverride, "baseCtorSource", "a generated child constructor must preserve the base constructor's receiver state")
	require.NotContains(t, inheritedOverride, "baseInitSource")
}

func TestCSharp_ConstructorStateSurvivesCallValueBoundaries(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class StateBox {
    public object Value = initSource();
    public StateBox() { Value = ctorSource(); }
}

public class StateReader {
    public static object Read(StateBox box) { return box.Value; }
}

public class StateFactory {
    public static StateBox Make() { return new StateBox(); }
}

public class Program {
    public static void Main() {
        parameterBoundarySink(StateReader.Read(new StateBox()));
        factoryBoundarySink(StateFactory.Make().Value);
    }
}
`)

	flow, err := prog.SyntaxFlowWithError(`
parameterBoundarySink(* #-> as $parameterBoundary);
factoryBoundarySink(* #-> as $factoryBoundary)
`)
	require.NoError(t, err)
	parameterBoundary := flow.GetValues("parameterBoundary").String()
	require.Contains(t, parameterBoundary, "ctorSource")
	require.NotContains(t, parameterBoundary, "initSource")
	factoryBoundary := flow.GetValues("factoryBoundary").String()
	require.Contains(t, factoryBoundary, "ctorSource")
	require.NotContains(t, factoryBoundary, "initSource")
}
