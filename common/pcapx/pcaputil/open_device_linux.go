package pcaputil

func deviceNameToPcapGuidWindows(wantName string) (string, error) {
	return "", NewConvertIfaceNameError(wantName)
}
