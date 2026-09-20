package analyzer

// IsExecutable recognizes only the executable containers accepted by this
// analyzer. It does not install a process-wide matcher in a filetype registry.
func IsExecutable(buf []byte) bool {
	return len(buf) >= 4 && (string(buf[:4]) == "\x7fELF" || buf[0] == 'M' && buf[1] == 'Z')
}
