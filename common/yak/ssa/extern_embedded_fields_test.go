package ssa

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type embeddedModel struct{ ID uint }
type embeddedFlow struct{ *embeddedModel }
type embeddedHistory struct{ *embeddedFlow }
type embeddedMetadata struct{ Label string }
type embeddedMultiple struct {
	embeddedHistory
	embeddedMetadata
}
type embeddedDirect struct {
	embeddedHistory
	ID string
}
type embeddedShallow struct {
	embeddedHistory
	embeddedOtherID
}
type embeddedOtherID struct{ ID string }
type embeddedAmbiguous struct {
	embeddedModel
	embeddedOtherID
}
type embeddedLeft struct{ embeddedModel }
type embeddedRight struct{ embeddedModel }
type embeddedDiamond struct {
	embeddedLeft
	embeddedRight
}
type embeddedCycle struct {
	*embeddedCycle
	embeddedHistory
}
type embeddedNamed struct{ Model embeddedModel }

func TestExternEmbeddedFieldResolution(t *testing.T) {
	// Compare SSA field lookup with Go reflection, including promotion depth,
	// shadowing, ambiguous paths to the same type, and recursive pointer types.
	for _, value := range []any{
		embeddedHistory{}, embeddedMultiple{}, embeddedDirect{}, embeddedShallow{},
		embeddedAmbiguous{}, embeddedDiamond{}, embeddedCycle{}, embeddedNamed{},
	} {
		typ := reflect.TypeOf(value)
		t.Run(typ.Name(), func(t *testing.T) {
			prog := NewTmpProgram(t.Name())
			object := prog.handlerType(typ, 0).(*ObjectType)
			// Every anonymous field must survive reflection import independently.
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				if field.Anonymous {
					require.Contains(t, object.AnonymousField, field.Name)
				}
			}
			// Include all visible fields and explicitly probe hidden/missing names.
			keys := map[string]bool{"ID": true, "Label": true, "Missing": true}
			for _, field := range reflect.VisibleFields(typ) {
				keys[field.Name] = true
			}
			for name := range keys {
				field, visible := typ.FieldByName(name)
				actual := object.GetField(NewConst(name))
				if !visible {
					require.Nil(t, actual, "Go does not expose %s.%s", typ.Name(), name)
					continue
				}
				require.NotNil(t, actual, "%s.%s", typ.Name(), name)
				expected := prog.handlerType(field.Type, 0)
				require.Equal(t, expected.GetTypeKind(), actual.GetTypeKind(), "%s.%s", typ.Name(), name)
				require.Equal(t, expected.String(), actual.String(), "%s.%s", typ.Name(), name)
			}
		})
	}
}
