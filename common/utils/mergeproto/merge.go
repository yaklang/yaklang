package mergeproto

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/yaklang/yaklang/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const templateContent = `// merge multiple proto files, DO NOT EDIT.

syntax = "proto3";

package ypb;
option go_package = "/;ypb";
`

const ServiceName = "Yak"

var (
	allMessageDescriptors = map[string]*descriptorpb.DescriptorProto{}
)

type packageFileGroup struct {
	name        string
	fullName    string
	files       []*descriptorpb.FileDescriptorProto
	subPackages map[string]*packageFileGroup
}

func GenProtoBytes(path, pPackage string) (*Buffer, error) {
	var entries []string
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	fmt.Println("Current directory:", dir)
	if err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return err
		}
		entries = append(entries, path)
		return nil
	}); err != nil {
		log.Fatalf("unable to walk dir: %v", err)
		return nil, err
	}

	parser := protoparse.Parser{
		IncludeSourceCodeInfo: true,
	}

	var filePaths []string
	for _, entry := range entries {
		filePaths = append(filePaths, filepath.Join(path, entry))
	}

	files, err := parser.ParseFilesButDoNotLink(filePaths...)
	if err != nil {
		log.Fatalf("unable to parse files: %v", err)
		return nil, err
	}

	buf := NewBuffer()

	buf.Printf(templateContent)

	// group files by package name
	pfg := &packageFileGroup{
		subPackages: map[string]*packageFileGroup{},
	}

	for _, file := range files {
		pkg := file.GetPackage()
		var paths []string
		if pkg != pPackage {
			paths = strings.Split(trimPackageFromName(pkg, pPackage), ".")
		}

		if len(paths) == 0 {
			pfg.files = append(pfg.files, file)
		} else {
			var currentGroup = pfg
			for i, p := range paths {
				if currentGroup.subPackages[p] == nil {
					currentGroup.subPackages[p] = &packageFileGroup{
						name:        p,
						fullName:    strings.Join(paths[:i+1], "."),
						subPackages: map[string]*packageFileGroup{},
					}
				}
				currentGroup = currentGroup.subPackages[p]
			}
			currentGroup.files = append(currentGroup.files, file)
		}

		for _, message := range file.GetMessageType() {
			fillAllMessageDescriptors(file.GetPackage(), pPackage, message)
		}

	}

	var iteratePackageGroup func(group *packageFileGroup, identLevel int)
	iteratePackageGroup = func(group *packageFileGroup, identLevel int) {
		// 初始化一个新的服务描述符
		newService := &descriptorpb.ServiceDescriptorProto{
			Name: proto.String(ServiceName),
		}

		serviceMap := make(map[string]bool)

		methodMap := make(map[string]*descriptorpb.MethodDescriptorProto)

		for _, file := range group.files {
			for _, service := range file.Service {
				// 如果该 service 名称在 serviceMap 中不存在
				if _, ok := serviceMap[*service.Name]; !ok {
					serviceMap[*service.Name] = true
					for _, method := range service.Method {
						// 如果该 rpc 方法名称在 methodMap 中不存在
						if _, ok := methodMap[*method.Name]; !ok {
							methodMap[*method.Name] = method
							newService.Method = append(newService.Method, method)
						}
					}
				}
			}
		}
		spew.Dump(newService)
		// 生成新的服务
		generateService(buf, 0, newService)

		// 这是合并为多个 Service
		//for _, file := range group.files {
		//	for _, service := range file.Service {
		//		generateService(buf, 0, service)
		//		buf.Printf("")
		//	}
		//}

		if group.name != "" {
			buf.Printf("%smessage %s {", strings.Repeat(" ", identLevel*4), group.name)
		}
		enumMap := make(map[string]*descriptorpb.EnumDescriptorProto)
		messageMap := make(map[string]*descriptorpb.DescriptorProto)

		for _, file := range group.files {
			for _, enum := range file.EnumType {
				// 如果该枚举名称在 enumMap 中不存在
				if _, ok := enumMap[*enum.Name]; !ok {
					enumMap[*enum.Name] = enum
					// 这里调用你的 generateEnum 函数，只有当枚举名称不重复时才会调用
					generateEnum(buf, identLevel+1, enum)
					buf.Printf("")
				}
			}

			for _, message := range file.GetMessageType() {

				// 如果该消息类型在 messageMap 中不存在
				if _, ok := messageMap[*message.Name]; !ok {
					messageMap[*message.Name] = message
					resolveMessageExtends(message, false, pPackage)
					generateMessage(buf, identLevel+1, message)
					buf.Printf("")
				}
			}

			generateExtensions(buf, identLevel+1, nil, file.GetExtension())
			buf.Printf("")
		}

		for _, subPackage := range group.subPackages {
			iteratePackageGroup(subPackage, identLevel+1)
		}
		if group.name != "" {
			buf.Printf("%s}", strings.Repeat(" ", identLevel*4))
		}
		buf.Printf("")
	}
	iteratePackageGroup(pfg, -1)

	return buf, nil
}

func resolveMessageExtends(message *descriptorpb.DescriptorProto, optionsFlag bool, pPackage string) {

	for _, nested := range message.GetNestedType() {
		resolveMessageExtends(nested, optionsFlag, pPackage)
	}

	var options []*descriptorpb.UninterpretedOption
	var keepOptions []*descriptorpb.UninterpretedOption
	var fields []*descriptorpb.FieldDescriptorProto
	for _, option := range message.GetOptions().GetUninterpretedOption() {
		if isOptionOneProtoExtends(option) {
			parent := allMessageDescriptors[trimPackageFromName(string(option.GetStringValue()), pPackage)]
			if parent == nil {
				log.Fatalf("unable to find message %s", option.GetStringValue())
			}
			resolveMessageExtends(parent, optionsFlag, pPackage)
			fields = append(fields, parent.Field...)
			options = append(options, parent.GetOptions().GetUninterpretedOption()...)
		} else {
			keepOptions = append(keepOptions, option)
		}
	}
	if message.Options != nil {
		message.Options.UninterpretedOption = keepOptions
	}

	message.Field = append(message.Field, fields...)
	if optionsFlag {
		if message.Options == nil {
			message.Options = &descriptorpb.MessageOptions{}
		}
		for i := range options {
			if isOptionOneProtoExtends(options[i]) {
				options = append(options[:i], options[i+1:]...)
			}
		}
	}
	if message.Options != nil {
		message.Options.UninterpretedOption = append(message.Options.UninterpretedOption, options...)
	}
	sort.Slice(message.Field, func(i, j int) bool {
		return message.Field[i].GetNumber() < message.Field[j].GetNumber()
	})
}

func fillAllMessageDescriptors(pkg, pPackage string, message *descriptorpb.DescriptorProto) {
	allMessageDescriptors[trimPackageFromName(fmt.Sprintf("%s.%s", pkg, message.GetName()), pPackage)] = message
	for _, sub := range message.GetNestedType() {
		fillAllMessageDescriptors(fmt.Sprintf("%s.%s", pkg, message.GetName()), pPackage, sub)
	}
}

func trimPackageFromName(name string, packageName string) string {
	return strings.TrimPrefix(name, packageName+".")
}

func isOptionOneProtoExtends(option *descriptorpb.UninterpretedOption) bool {
	if names := option.GetName(); len(names) > 0 && names[0].GetNamePart() == "oneproto.extends" {
		return true
	}
	return false
}
