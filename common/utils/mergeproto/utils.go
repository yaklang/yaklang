package mergeproto

import (
	"fmt"
	"google.golang.org/protobuf/types/descriptorpb"
	"strconv"
	"strings"
)

func generateService(buf *Buffer, indentLevel int, service *descriptorpb.ServiceDescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%sservice %s {", indent, service.GetName())
	generateHeadOptions(buf, indent, service.GetOptions().GetUninterpretedOption())
	for _, method := range service.Method {
		buf.Printf("%s    rpc %s(%s) returns (%s) {", indent, method.GetName(), method.GetInputType(), method.GetOutputType())
		for _, opt := range method.GetOptions().GetUninterpretedOption() {
			buf.Printf("%s        option %s;", indent, StringifyUninterpretedOption(opt))
		}
		buf.Printf("%s    }", indent)
		buf.Printf("")
	}
	buf.Printf("%s}", indent)
}

func generateEnum(buf *Buffer, indentLevel int, enum *descriptorpb.EnumDescriptorProto) {
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%senum %s {", indent, enum.GetName())
	generateHeadOptions(buf, indent, enum.GetOptions().GetUninterpretedOption())

	for _, value := range enum.Value {
		buf.Printf("%s    %s  = %d%s;", indent, value.GetName(), value.GetNumber(), StringifyValueOptions(value.GetOptions().GetUninterpretedOption()))
	}
	buf.Printf("%s}", indent)
}

func generateMessage(buf *Buffer, indentLevel int, message *descriptorpb.DescriptorProto) {
	//spew.Dump(allMessageDescriptors)
	indent := strings.Repeat(" ", indentLevel*4)
	buf.Printf("%smessage %s {", indent, message.GetName())
	generateHeadOptions(buf, indent, message.GetOptions().GetUninterpretedOption())
	for _, field := range message.GetField() {
		buf.Printf("%s    %s %s = %d%s;", indent, StringifyField(message, field), field.GetName(), field.GetNumber(), StringifyValueOptions(field.GetOptions().GetUninterpretedOption()))
	}

	for _, enum := range message.GetEnumType() {
		buf.Printf("")
		generateEnum(buf, indentLevel+1, enum)
	}

	for _, nested := range message.GetNestedType() {
		if nested.GetOptions().GetMapEntry() {
			continue
		}
		buf.Printf("")
		generateMessage(buf, indentLevel+1, nested)
	}

	buf.Printf("")
	generateExtensions(buf, indentLevel+1, message, message.GetExtension())
	buf.Printf("%s}", indent)
}

func generateHeadOptions(buf *Buffer, indent string, options []*descriptorpb.UninterpretedOption) {
	if len(options) == 0 {
		return
	}
	for _, opt := range options {
		buf.Printf("%s    option %s;", indent, StringifyUninterpretedOption(opt))
	}
	buf.Printf("")
}

func generateExtensions(buf *Buffer, indentLevel int, message *descriptorpb.DescriptorProto, extensions []*descriptorpb.FieldDescriptorProto) {
	grouped := map[string][]*descriptorpb.FieldDescriptorProto{}
	for _, ext := range extensions {
		grouped[ext.GetExtendee()] = append(grouped[ext.GetExtendee()], ext)
	}
	indent := strings.Repeat(" ", indentLevel*4)
	for extendee, exts := range grouped {

		buf.Printf("%sextend %s {", indent, extendee)
		for _, ext := range exts {
			buf.Printf("%s    %s %s = %d%s;", indent, StringifyField(message, ext), ext.GetName(), ext.GetNumber(), StringifyValueOptions(ext.GetOptions().GetUninterpretedOption()))
		}
		buf.Printf("%s}", indent)
	}
}

func StringifyUninterpretedOption(opt *descriptorpb.UninterpretedOption) string {
	var value string
	if v := opt.IdentifierValue; v != nil {
		value = *v
	} else if v := opt.DoubleValue; v != nil {
		value = strconv.FormatFloat(*v, 'f', -1, 64)
	} else if v := opt.AggregateValue; v != nil {
		value = fmt.Sprintf("{%s}", *v)
	} else if v := opt.StringValue; v != nil {
		value = fmt.Sprintf("'%s'", v)
	} else if v := opt.PositiveIntValue; v != nil {
		value = strconv.Itoa(int(*v))
	} else if v := opt.NegativeIntValue; v != nil {
		value = strconv.Itoa(int(*v))
	}
	var nameParts []string
	for _, name := range opt.GetName() {
		nameParts = append(nameParts, name.GetNamePart())
	}

	return fmt.Sprintf("(%s) = %s", strings.Join(nameParts, ","), value)
}

func StringifyField(message *descriptorpb.DescriptorProto, field *descriptorpb.FieldDescriptorProto) string {
	repeated := field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	if field.Type == nil {
		name := field.GetTypeName()
		for _, nested := range message.GetNestedType() {
			if name == nested.GetName() && strings.HasSuffix(name, "Entry") { // map entry
				return fmt.Sprintf("map<%s,%s>", StringifyField(nested, nested.Field[0]), StringifyField(nested, nested.Field[1]))
			}
		}
		if repeated {
			return "repeated " + name
		}
		return name
	}
	name := strings.ToLower(field.GetType().String()[5:])
	if repeated {
		name = "repeated " + name
	}
	return name
}

func StringifyValueOptions(options []*descriptorpb.UninterpretedOption) string {
	if len(options) == 0 {
		return ""
	}
	var opts []string
	for _, opt := range options {
		opts = append(opts, StringifyUninterpretedOption(opt))
	}
	return fmt.Sprintf(" [%s]", strings.Join(opts, ", "))
}
