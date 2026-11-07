package pcaputil

import (
	"fmt"
	"strings"
)

// SMB observations are protocol facts. This file derives analyst-facing
// operation and behavior labels from already-decoded fields; it never infers
// authentication, intent or success that the wire did not carry.

func smbStatusName(status uint32) string {
	switch status {
	case 0x00000000:
		return "STATUS_SUCCESS"
	case 0x00000103:
		return "STATUS_PENDING"
	case 0x0000010e:
		return "STATUS_NOTIFY_ENUM_DIR"
	case 0x80000005:
		return "STATUS_BUFFER_OVERFLOW"
	case 0x80000006:
		return "STATUS_NO_MORE_FILES"
	case 0xc0000016:
		return "STATUS_MORE_PROCESSING_REQUIRED"
	case 0xc0000022:
		return "STATUS_ACCESS_DENIED"
	case 0xc0000034:
		return "STATUS_OBJECT_NAME_NOT_FOUND"
	case 0xc0000035:
		return "STATUS_OBJECT_NAME_COLLISION"
	case 0xc0000043:
		return "STATUS_SHARING_VIOLATION"
	case 0xc000006d:
		return "STATUS_LOGON_FAILURE"
	case 0xc0000072:
		return "STATUS_ACCOUNT_DISABLED"
	case 0xc00000bb:
		return "STATUS_NOT_SUPPORTED"
	case 0xc00000cc:
		return "STATUS_BAD_NETWORK_NAME"
	case 0xc0000203:
		return "STATUS_USER_SESSION_DELETED"
	}
	return fmt.Sprintf("STATUS_0x%08X", status)
}

func smb2OperationName(command uint16) string {
	switch command {
	case 0:
		return "negotiate"
	case 1:
		return "session.setup"
	case 2:
		return "session.logoff"
	case 3:
		return "share.connect"
	case 4:
		return "share.disconnect"
	case 5:
		return "file.open"
	case 6:
		return "file.close"
	case 7:
		return "file.flush"
	case 8:
		return "file.read"
	case 9:
		return "file.write"
	case 10:
		return "file.lock"
	case 11:
		return "ioctl"
	case 12:
		return "request.cancel"
	case 13:
		return "echo"
	case 14:
		return "directory.enumerate"
	case 15:
		return "file.change_notify"
	case 16:
		return "file.query_info"
	case 17:
		return "file.set_info"
	case 18:
		return "file.oplock_break"
	}
	return fmt.Sprintf("command.%d", command)
}

func smb1OperationName(command byte, body map[string]any) string {
	switch command {
	case 0x72:
		return "negotiate"
	case 0x73:
		return "session.setup"
	case 0x74:
		return "session.logoff"
	case 0x75:
		return "share.connect"
	case 0x06:
		return "file.delete"
	case 0x22:
		return "file.set_info"
	case 0x23:
		return "file.query_info"
	case 0x2e:
		return "file.read"
	case 0x2f:
		return "file.write"
	case 0x04:
		return "file.close"
	case 0xa2:
		return "file.open"
	case 0x32:
		if setup, ok := body["Setup"].(uint16); ok {
			switch setup {
			case 0x0001, 0x0002, 0x0005, 0x0006, 0x0007:
				return "transaction2.path_info"
			case 0x0008:
				return "transaction2.file_info"
			case 0x0009:
				return "transaction2.fs_info"
			case 0x000a:
				return "transaction2.query_fs_size"
			}
		}
		return "transaction2"
	case 0x33:
		return "transaction2.secondary"
	case 0xa0, 0xa1:
		return "transaction.nt"
	}
	return fmt.Sprintf("smb1.command.0x%02x", command)
}

func smbIOCTLName(code uint32) string {
	switch code {
	case smb2FSCTLQueryNetworkInterfaceInfo:
		return "FSCTL_QUERY_NETWORK_INTERFACE_INFO"
	case smb2FSCTLDFSGetReferrals:
		return "FSCTL_DFS_GET_REFERRALS"
	case smb2FSCTLPipeTransceive:
		return "FSCTL_PIPE_TRANSCEIVE"
	}
	return fmt.Sprintf("FSCTL_0x%08X", code)
}

func smb2InfoClassName(infoType, infoClass uint8) string {
	switch {
	case infoType == 1 && infoClass == 4:
		return "FileBasicInformation"
	case infoType == 1 && infoClass == 5:
		return "FileStandardInformation"
	case infoType == 1 && infoClass == 6:
		return "FileInternalInformation"
	case infoType == 1 && infoClass == 7:
		return "FileEaInformation"
	case infoType == 1 && infoClass == 22:
		return "FileStreamInformation"
	case infoType == 1 && infoClass == 34:
		return "FileNetworkOpenInformation"
	case infoType == 1 && infoClass == 35:
		return "FileAttributeTagInformation"
	case infoType == 2 && infoClass == 1:
		return "FileFsVolumeInformation"
	case infoType == 2 && infoClass == 5:
		return "FileFsAttributeInformation"
	case infoType == 3 && infoClass == 0:
		return "FileSecurityInformation"
	}
	return fmt.Sprintf("InfoType_%d_Class_%d", infoType, infoClass)
}

func smbShareKind(path string) string {
	if strings.EqualFold(strings.TrimSpace(path), "IPC$") || strings.HasSuffix(strings.ToUpper(strings.TrimSpace(path)), `\IPC$`) {
		return "named-pipe"
	}
	return "file-share"
}

func smbPipeService(path string) string {
	lower := strings.ToLower(path)
	for _, item := range []struct {
		fragment, service string
	}{
		{"svcctl", "service-control"},
		{"winreg", "remote-registry"},
		{"samr", "account-management"},
		{"lsarpc", "local-security-authority"},
		{"srvsvc", "server-service"},
		{"netlogon", "netlogon"},
		{"atsvc", "task-scheduler"},
		{"eventlog", "event-log"},
		{"spoolss", "print-spooler"},
		{"wkssvc", "workstation-service"},
	} {
		if strings.Contains(lower, item.fragment) {
			return item.service
		}
	}
	return ""
}

func smb2AccessIntent(body map[string]any) []string {
	access, ok := body["Desired Access"].(uint32)
	if !ok {
		return nil
	}
	var out []string
	add := func(bit uint32, name string) {
		if access&bit != 0 {
			out = append(out, name)
		}
	}
	add(0x80000000, "generic-read")
	add(0x40000000, "generic-write")
	add(0x20000000, "generic-execute")
	add(0x10000000, "generic-all")
	add(0x00000001, "read-data")
	add(0x00000002, "write-data")
	add(0x00000004, "append-data")
	add(0x00000008, "read-ea")
	add(0x00000010, "write-ea")
	add(0x00000020, "execute")
	add(0x00000040, "delete-child")
	add(0x00000080, "read-attributes")
	add(0x00000100, "write-attributes")
	add(0x00010000, "delete")
	add(0x00020000, "read-control")
	add(0x00040000, "write-dac")
	add(0x00080000, "write-owner")
	add(0x00100000, "synchronize")
	return out
}

func smb2CreateOptions(body map[string]any) []string {
	options, ok := body["Create Options"].(uint32)
	if !ok {
		return nil
	}
	var out []string
	add := func(bit uint32, name string) {
		if options&bit != 0 {
			out = append(out, name)
		}
	}
	add(0x00000001, "directory")
	add(0x00000040, "non-directory")
	add(0x00001000, "delete-on-close")
	add(0x00002000, "open-by-file-id")
	add(0x00200000, "open-reparse-point")
	add(0x01000000, "no-recall-open")
	return out
}

func smbHasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func smb2CreateDisposition(body map[string]any) string {
	disposition, ok := body["Create Disposition"].(uint32)
	if !ok {
		return ""
	}
	switch disposition {
	case 0:
		return "supersede"
	case 1:
		return "open"
	case 2:
		return "create"
	case 3:
		return "open-if"
	case 4:
		return "overwrite"
	case 5:
		return "overwrite-if"
	}
	return fmt.Sprintf("disposition.%d", disposition)
}

func smbDedupStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := values[:0]
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// smb2AttachContext adds analyst-facing operation, resource and behavior
// labels. It is idempotent so an associated response can be enriched with the
// matched request's path without duplicating labels.
func smb2AttachContext(info map[string]any) {
	if info == nil {
		return
	}
	command, _ := info["Command"].(uint16)
	body, _ := info["Body"].(map[string]any)
	info["Operation"] = smb2OperationName(command)
	info["Operation Category"] = strings.SplitN(smb2OperationName(command), ".", 2)[0]
	if status, ok := info["Status"].(uint32); ok {
		info["Status Name"] = smbStatusName(status)
	}
	path, _ := body["Path"].(string)
	if path == "" {
		path, _ = info["Request Path"].(string)
	}
	file, _ := body["File Name"].(string)
	if file == "" {
		file, _ = info["Request File Name"].(string)
	}
	if file == "" {
		file, _ = info["File Context"].(string)
	}
	behaviors := []string{}
	add := func(value string) { behaviors = append(behaviors, value) }
	resource := ""
	switch command {
	case 1:
		add("authentication")
		resource = "session"
	case 3:
		if path != "" {
			resource = path
			info["Resource Kind"] = smbShareKind(path)
			if info["Resource Kind"] == "named-pipe" {
				add("named-pipe-access")
				add("rpc-endpoint")
			} else {
				add("share-connect")
			}
		}
	case 5:
		if file != "" {
			resource = file
			if strings.HasPrefix(strings.ToLower(file), `\pipe\`) || strings.Contains(strings.ToLower(file), "pipe") {
				add("named-pipe-access")
				info["Resource Kind"] = "named-pipe"
				if service := smbPipeService(file); service != "" {
					add(service)
				} else {
					add("rpc-endpoint")
				}
			} else {
				options := smb2CreateOptions(body)
				if smbHasString(options, "directory") {
					add("directory-open")
					info["Resource Kind"] = "directory"
				} else {
					add("file-open")
					info["Resource Kind"] = "file"
				}
			}
			info["Access Intent"] = smb2AccessIntent(body)
			info["Create Options Names"] = smb2CreateOptions(body)
			if disposition := smb2CreateDisposition(body); disposition != "" {
				info["Create Disposition Name"] = disposition
			}
		}
	case 6:
		add("file-close")
	case 7:
		add("file-flush")
	case 8:
		add("file-read")
	case 9:
		add("file-write")
	case 10:
		add("file-lock")
	case 11:
		if body != nil {
			if code, ok := body["Ctl Code"].(uint32); ok {
				info["Control Name"] = smbIOCTLName(code)
				switch code {
				case smb2FSCTLDFSGetReferrals:
					add("dfs-referral")
					resource = "dfs"
					info["Resource Kind"] = "dfs"
				case smb2FSCTLQueryNetworkInterfaceInfo:
					add("network-interface-enumeration")
					resource = "network-interface"
					info["Resource Kind"] = "network-interface"
				case smb2FSCTLPipeTransceive:
					add("named-pipe-transceive")
					resource = "named-pipe"
					info["Resource Kind"] = "named-pipe"
				}
			}
		}
	case 14:
		add("directory-enumerate")
		if path != "" {
			resource = path
		}
		info["Resource Kind"] = "directory"
	case 15:
		add("file-change-notify")
	case 16:
		if body != nil {
			infoType, _ := body["Info Type"].(uint8)
			infoClass, _ := body["File Info Class"].(uint8)
			if infoType == 0 && infoClass == 0 {
				infoType, _ = info["Request Info Type"].(uint8)
				infoClass, _ = info["Request File Info Class"].(uint8)
			}
			if infoType != 0 || infoClass != 0 {
				info["Information Class Name"] = smb2InfoClassName(infoType, infoClass)
			}
			if infoType == 3 && infoClass == 0 {
				add("security-descriptor-query")
				resource = "security-descriptor"
				info["Resource Kind"] = "security-descriptor"
			} else if infoType == 1 && infoClass == 22 {
				add("alternate-data-stream-query")
			} else {
				add("file-metadata-query")
			}
		}
	case 17:
		add("file-metadata-set")
	case 18:
		add("file-oplock-break")
	case 2:
		add("session-logoff")
	case 4:
		add("share-disconnect")
	}
	info["Observed Behaviors"] = smbDedupStrings(behaviors)
	if resource != "" {
		info["Resource"] = resource
		if _, ok := info["Resource Kind"]; !ok {
			info["Resource Kind"] = "path"
		}
	}
}

func smb1AttachContext(fields map[string]any, command byte) {
	if fields == nil {
		return
	}
	body, _ := fields["Body"].(map[string]any)
	if body == nil {
		body = map[string]any{}
	}
	fields["Operation"] = smb1OperationName(command, body)
	fields["Operation Category"] = strings.SplitN(fields["Operation"].(string), ".", 2)[0]
	response, _ := fields["Response"].(bool)
	if header, ok := fields["Header"].(map[string]any); ok {
		if flags, ok := header["Flags"].(byte); ok {
			response = flags&0x80 != 0
		}
		if status, ok := header["Status"].(uint32); ok && response {
			// Derived text, not a wire literal: keep the Header map limited to
			// its observed fields and ranges.
			fields["Status Name"] = smbStatusName(status)
		}
	}
	path, _ := body["Path"].(string)
	if path == "" {
		path, _ = fields["Request Path"].(string)
	}
	file, _ := body["File Name"].(string)
	if file == "" {
		file, _ = fields["Request File Name"].(string)
	}
	if file == "" {
		file, _ = fields["Reassembled File Name"].(string)
	}
	behaviors := []string{}
	add := func(value string) { behaviors = append(behaviors, value) }
	resource := ""
	switch command {
	case 0x73:
		add("authentication")
		resource = "session"
	case 0x75:
		if path != "" {
			resource = path
			if smbShareKind(path) == "named-pipe" {
				add("named-pipe-access")
				add("rpc-endpoint")
			} else {
				add("share-connect")
			}
		}
	case 0xa2:
		if file != "" {
			resource = file
			if strings.HasPrefix(strings.ToLower(file), `\pipe\`) {
				add("named-pipe-access")
				if service := smbPipeService(file); service != "" {
					add(service)
				} else {
					add("rpc-endpoint")
				}
			} else {
				add("file-open")
			}
			if intent := smb2AccessIntent(body); len(intent) > 0 {
				fields["Access Intent"] = intent
			}
		}
	case 0x2e:
		add("file-read")
	case 0x2f:
		add("file-write")
	case 0x06:
		add("file-delete")
	case 0x04:
		add("file-close")
	case 0x22:
		add("file-metadata-set")
	case 0x23:
		add("file-metadata-query")
	case 0x32:
		if setup, ok := body["Setup"].(uint16); ok {
			switch setup {
			case 0x0005:
				add("file-metadata-query")
			case 0x0006:
				add("file-metadata-set")
			case 0x0001, 0x0002:
				add("directory-enumerate")
			}
		}
	}
	fields["Observed Behaviors"] = smbDedupStrings(behaviors)
	if resource != "" {
		fields["Resource"] = resource
	}
}
