package ypb_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// The old clients know only the numeric field. Exercise both directions of
// the wire exchange instead of merely checking the Go field's type.
func TestSSARiskResultUUIDWireCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message proto.Message
		number  int32
		list    bool
	}{
		{"risk", &ypb.SSARisk{ResultID: 42, ResultUUID: "result-uuid"}, 22, false},
		{"filter", &ypb.SSARisksFilter{ResultID: []uint64{42, 99}, ResultUUID: []string{"result-uuid"}}, 9, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := tc.message.ProtoReflect().Descriptor().Fields()
			id := fields.ByName("ResultID")
			require.Equal(t, protoreflect.Uint64Kind, id.Kind())
			require.Equal(t, protoreflect.FieldNumber(tc.number), id.Number())
			require.Equal(t, tc.list, id.IsList())
			require.Equal(t, protoreflect.StringKind, fields.ByName("ResultUUID").Kind())

			label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
			if tc.list {
				label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
			}
			file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
				Name: proto.String("legacy.proto"), Syntax: proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{{
					Name: proto.String("LegacyRisk"),
					Field: []*descriptorpb.FieldDescriptorProto{{
						Name: proto.String("ResultID"), Number: proto.Int32(tc.number),
						Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(), Label: &label,
					}},
				}},
			}, nil)
			require.NoError(t, err)
			legacy := dynamicpb.NewMessage(file.Messages().Get(0))
			wire, err := proto.Marshal(tc.message)
			require.NoError(t, err)
			require.NoError(t, (proto.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(wire, legacy))
			oldID := legacy.Descriptor().Fields().ByNumber(protoreflect.FieldNumber(tc.number))
			if tc.list {
				require.Equal(t, uint64(42), legacy.Get(oldID).List().Get(0).Uint())
				require.Equal(t, uint64(99), legacy.Get(oldID).List().Get(1).Uint())
			} else {
				require.Equal(t, uint64(42), legacy.Get(oldID).Uint())
			}

			wire, err = proto.Marshal(legacy)
			require.NoError(t, err)
			newMessage := tc.message.ProtoReflect().New().Interface()
			require.NoError(t, proto.Unmarshal(wire, newMessage))
			if tc.list {
				got := newMessage.ProtoReflect().Get(id).List()
				require.Equal(t, 2, got.Len())
				require.Equal(t, uint64(42), got.Get(0).Uint())
				require.Equal(t, uint64(99), got.Get(1).Uint())
			} else {
				require.Equal(t, uint64(42), newMessage.ProtoReflect().Get(id).Uint())
			}
			require.False(t, newMessage.ProtoReflect().Has(fields.ByName("ResultUUID")))
		})
	}
}
