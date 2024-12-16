package javaclassparser

// 用于解析类的访问标志
func getClassAccessFlagsVerbose(u uint16) []string {
	result := []string{}
	maskMap := map[uint16]string{
		0x0001: "public", // ACC_PUBLIC
		0x0010: "final",  // ACC_FINAL
		//0x0020: "super",      // ACC_SUPER
		0x0200: "interface", // ACC_INTERFACE
		0x0400: "abstract",  // ACC_ABSTRACT
		//0x1000: "synthetic",  // ACC_SYNTHETIC
		0x2000: "annotation", // ACC_ANNOTATION
		0x4000: "enum",       // ACC_ENUM
	}
	for k, v := range maskMap {
		if u&k == k {
			result = append(result, v)
		}
	}
	return result
}

// 用于解析方法的访问标志
func getMethodAccessFlagsVerbose(u uint16) []string {
	result := []string{}
	maskMap := map[uint16]string{
		0x0001: "public",       // ACC_PUBLIC
		0x0002: "private",      // ACC_PRIVATE
		0x0004: "protected",    // ACC_PROTECTED
		0x0008: "static",       // ACC_STATIC
		0x0010: "final",        // ACC_FINAL
		0x0020: "synchronized", // ACC_SYNCHRONIZED
		//0x0040: "bridge",       // ACC_BRIDGE
		0x0080: "varargs",  // ACC_VARARGS
		0x0100: "native",   // ACC_NATIVE
		0x0400: "abstract", // ACC_ABSTRACT
		0x0800: "strict",   // ACC_STRICT
		//0x1000: "synthetic",    // ACC_SYNTHETIC
	}
	for k, v := range maskMap {
		if u&k == k {
			result = append(result, v)
		}
	}
	return result
}

// 用于解析字段的访问标志
func getFieldAccessFlagsVerbose(u uint16) []string {
	result := []string{}
	maskMap := map[uint16]string{
		0x0001: "public",    // ACC_PUBLIC
		0x0002: "private",   // ACC_PRIVATE
		0x0004: "protected", // ACC_PROTECTED
		0x0008: "static",    // ACC_STATIC
		0x0010: "final",     // ACC_FINAL
		0x0040: "volatile",  // ACC_VOLATILE
		0x0080: "transient", // ACC_TRANSIENT
		//0x1000: "synthetic", // ACC_SYNTHETIC
		0x4000: "enum", // ACC_ENUM
	}
	for k, v := range maskMap {
		if u&k == k {
			result = append(result, v)
		}
	}
	return result
}
