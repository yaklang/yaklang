package javaclassparser

import (
	"bytes"
	"strings"
)

// 用于解析类的访问标志
func getClassAccessFlagsVerbose(u uint16) ([]string, string) {
	result := []string{}
	var target bytes.Buffer

	// 按照访问标志的优先级顺序定义掩码映射
	masks := []uint16{
		0x0001, // public
		0x0010, // final
		0x0200, // interface
		0x0400, // abstract
		0x2000, // annotation
		0x4000, // enum
	}

	maskMap := map[uint16]string{
		0x0001: "public", // ACC_PUBLIC
		0x0010: "final",  // ACC_FINAL
		//0x0020: "super",    // ACC_SUPER
		0x0200: "interface", // ACC_INTERFACE
		0x0400: "abstract",  // ACC_ABSTRACT
		//0x1000: "synthetic",// ACC_SYNTHETIC
		0x2000: "annotation", // ACC_ANNOTATION
		0x4000: "enum",       // ACC_ENUM
	}

	isInterface := false
	isEnum := false
	// 按照预定义的顺序检查访问标志
	for _, mask := range masks {
		if u&mask == mask {
			verbose := maskMap[mask]

			if mask == 0x0200 { // interface
				result = append(result, verbose)
				target.WriteString(verbose)
				target.WriteByte(' ')
				isInterface = true
				break // 如果是 interface，则直接跳出，不再添加其他修饰符
			} else if mask == 0x4000 { // enum
				result = append(result, verbose)
				target.WriteString(verbose)
				target.WriteByte(' ')
				isEnum = true
			} else if mask == 0x0400 && isEnum {
				// 如果已经是枚举类，则不能是 abstract
				continue
			} else if (mask == 0x0400 || mask == 0x0010) && (isInterface) {
				// 如果是 interface，则不能是 abstract 或者 final
				continue
			} else if mask == 0x2000 && (isInterface || isEnum) {
				// 如果是 interface 或者 enum，则不能是 annotation
				continue
			} else {
				result = append(result, verbose)
				target.WriteString(verbose)
				target.WriteByte(' ')
			}
		}
	}

	return result, strings.TrimSpace(target.String())
}

// 用于解析方法的访问标志
func getMethodAccessFlagsVerbose(u uint16) ([]string, string) {
	result := []string{}
	var target bytes.Buffer

	// 按访问权限、静态、final、synchronized等顺序定义掩码
	masks := []uint16{
		0x0001, // public
		0x0004, // protected
		0x0002, // private
		0x0008, // static
		0x0010, // final
		0x0020, // synchronized
		0x0080, // varargs
		0x0100, // native
		0x0400, // abstract
		0x0800, // strict
	}

	maskMap := map[uint16]string{
		0x0001: "public",       // ACC_PUBLIC
		0x0002: "private",      // ACC_PRIVATE
		0x0004: "protected",    // ACC_PROTECTED
		0x0008: "static",       // ACC_STATIC
		0x0010: "final",        // ACC_FINAL
		0x0020: "synchronized", // ACC_SYNCHRONIZED
		0x0080: "varargs",      // ACC_VARARGS
		0x0100: "native",       // ACC_NATIVE
		0x0400: "abstract",     // ACC_ABSTRACT
		0x0800: "strict",       // ACC_STRICT
	}

	isAbstract := false
	isNative := false

	for _, mask := range masks {
		if u&mask == mask {
			verbose := maskMap[mask]

			if mask == 0x0001 || mask == 0x0002 || mask == 0x0004 {
				// 访问修饰符(public/private/protected)互斥,只取第一个匹配的
				if len(result) > 0 && (result[0] == "public" || result[0] == "private" || result[0] == "protected") {
					continue
				}
			} else if mask == 0x0400 { // abstract
				isAbstract = true
			} else if mask == 0x0100 { // native
				isNative = true
			} else if mask == 0x0010 && (isAbstract || isNative) {
				// abstract或native方法不能是final
				continue
			}

			result = append(result, verbose)
			target.WriteString(verbose)
			target.WriteByte(' ')
		}
	}

	return result, strings.TrimSpace(target.String())
}

// 用于解析字段的访问标志
func getFieldAccessFlagsVerbose(u uint16) ([]string, string) {
	result := []string{}
	target := strings.Builder{}

	masks := []uint16{
		0x0001, // public
		0x0002, // private
		0x0004, // protected
		0x0008, // static
		0x0010, // final
		0x0040, // volatile
		0x0080, // transient
		0x4000, // enum
	}

	maskMap := map[uint16]string{
		0x0001: "public",    // ACC_PUBLIC
		0x0002: "private",   // ACC_PRIVATE
		0x0004: "protected", // ACC_PROTECTED
		0x0008: "static",    // ACC_STATIC
		0x0010: "final",     // ACC_FINAL
		0x0040: "volatile",  // ACC_VOLATILE
		0x0080: "transient", // ACC_TRANSIENT
		0x4000: "enum",      // ACC_ENUM
	}

	for _, mask := range masks {
		if u&mask == mask {
			verbose := maskMap[mask]

			if mask == 0x0001 || mask == 0x0002 || mask == 0x0004 {
				// 访问修饰符(public/private/protected)互斥,只取第一个匹配的
				if len(result) > 0 && (result[0] == "public" || result[0] == "private" || result[0] == "protected") {
					continue
				}
			}

			result = append(result, verbose)
			target.WriteString(verbose)
			target.WriteByte(' ')
		}
	}

	return result, strings.TrimSpace(target.String())
}
