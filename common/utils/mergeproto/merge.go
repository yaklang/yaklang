package mergeproto

import (
	"fmt"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/types/descriptorpb"
	_ "google.golang.org/protobuf/types/descriptorpb"
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

var (
	allMessageDescriptors = map[string]*descriptorpb.DescriptorProto{}
	parentResolvedMap     = map[*descriptorpb.DescriptorProto]bool{}
)

type PackageFileGroup struct {
	name        string
	fullName    string
	files       []*descriptorpb.FileDescriptorProto
	subPackages map[string]*PackageFileGroup
}

func GenerateProto(includePath, outputPath, packageName string,
	enableOptions bool, inputs []string) error {

	var entries []string
	for _, input := range inputs {
		if err := fs.WalkDir(os.DirFS(input), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".proto") {
				return err
			}
			entries = append(entries, path)
			return nil
		}); err != nil {
			return err
		}
	}

	parser := protoparse.Parser{
		IncludeSourceCodeInfo: true,
	}
	files, err := parser.ParseFilesButDoNotLink(lo.Map(entries, func(entry string, index int) string {
		return filepath.Join(includePath, entry)
	})...)
	if err != nil {
		return err
	}

	buf := NewBuffer()
	buf.Printf(string(templateContent))

	// group files by package name
	packageFileGroup := &PackageFileGroup{
		subPackages: map[string]*PackageFileGroup{},
	}
	for _, file := range files {
		pkg := file.GetPackage()
		var paths []string
		if pkg != *pPackage {
			paths = strings.Split(trimPackageFromName(pkg), ".")
		}

		if len(paths) == 0 {
			packageFileGroup.files = append(packageFileGroup.files, file)
		} else {
			var currentGroup = packageFileGroup
			for i, path := range paths {
				if currentGroup.subPackages[path] == nil {
					currentGroup.subPackages[path] = &PackageFileGroup{
						name:        path,
						fullName:    strings.Join(paths[:i+1], "."),
						subPackages: map[string]*PackageFileGroup{},
					}
				}
				currentGroup = currentGroup.subPackages[path]
			}
			currentGroup.files = append(currentGroup.files, file)
		}

		for _, message := range file.GetMessageType() {
			fillAllMessageDescriptors(file.GetPackage(), message)
		}
	}

	var iteratePackageGroup func(group *PackageFileGroup, identLevel int)
	iteratePackageGroup = func(group *PackageFileGroup, identLevel int) {
		// 初始化一个新的服务描述符
		newService := &descriptorpb.ServiceDescriptorProto{
			Name: proto.String("Yak"),
		}

		for _, file := range group.files {
			for _, service := range file.Service {
				// 将每个服务中的方法添加到新的服务描述符中
				newService.Method = append(newService.Method, service.Method...)
			}
		}

		// 生成新的服务
		oneproto_util.GenerateService(buf, 0, newService)

		//for _, file := range group.files {
		//	for _, service := range file.Service {
		//		oneproto_util.GenerateService(buf, 0, service)
		//		buf.Printf("")
		//	}
		//}

		if group.name != "" {
			buf.Printf("%smessage %s {", strings.Repeat(" ", identLevel*4), group.name)
		}

		for _, file := range group.files {
			for _, enum := range file.EnumType {
				oneproto_util.GenerateEnum(buf, identLevel+1, enum)
				buf.Printf("")
			}

			for _, message := range file.GetMessageType() {
				resolveMessageExtends(message)
				oneproto_util.GenerateMessage(buf, identLevel+1, message)
				buf.Printf("")
			}

			oneproto_util.GenerateExtensions(buf, identLevel+1, nil, file.GetExtension())
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
	iteratePackageGroup(packageFileGroup, -1)

	// at the end, instead of exiting the program, return the error (if any)
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

func resolveMessageExtends(message *descriptorpb.DescriptorProto) {
	if parentResolvedMap[message] {
		return
	}
	for _, nested := range message.GetNestedType() {
		resolveMessageExtends(nested)
	}

	parentResolvedMap[message] = true
	var options []*descriptorpb.UninterpretedOption
	var keepOptions []*descriptorpb.UninterpretedOption
	var fields []*descriptorpb.FieldDescriptorProto
	for _, option := range message.GetOptions().GetUninterpretedOption() {
		if isOptionOneProtoExtends(option) {
			parent := allMessageDescriptors[trimPackageFromName(string(option.GetStringValue()))]
			if parent == nil {
				log.Fatalf("unable to find message %s", option.GetStringValue())
			}
			resolveMessageExtends(parent)
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
	if *pOptions {
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

func trimPackageFromName(name string) string {
	return strings.TrimPrefix(name, *pPackage+".")
}

func isOptionOneProtoExtends(option *descriptorpb.UninterpretedOption) bool {
	if names := option.GetName(); len(names) > 0 && names[0].GetNamePart() == "oneproto.extends" {
		return true
	}
	return false
}

func fillAllMessageDescriptors(pkg string, message *descriptorpb.DescriptorProto) {
	allMessageDescriptors[trimPackageFromName(fmt.Sprintf("%s.%s", pkg, message.GetName()))] = message
	for _, sub := range message.GetNestedType() {
		fillAllMessageDescriptors(fmt.Sprintf("%s.%s", pkg, message.GetName()), sub)
	}
}
