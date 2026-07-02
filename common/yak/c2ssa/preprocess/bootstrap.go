package preprocess

// BootstrapDefines returns default predefined macros for common C project trees.
// OpenSSL unconfigured .h.in templates rely on version/API macros for #if guards.
func BootstrapDefines() map[string]string {
	return map[string]string{
		"OPENSSL_VERSION_MAJOR":  "3",
		"OPENSSL_VERSION_MINOR":  "4",
		"OPENSSL_VERSION_PATCH":  "0",
		"OPENSSL_CONFIGURED_API": "30400",
		"OPENSSL_API_LEVEL":      "30400",
	}
}

func mergeBootstrapDefines(defs map[string]string) map[string]string {
	out := make(map[string]string, len(defs)+len(BootstrapDefines()))
	for k, v := range BootstrapDefines() {
		out[k] = v
	}
	for k, v := range defs {
		out[k] = v
	}
	return out
}
