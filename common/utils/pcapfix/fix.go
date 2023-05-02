package pcapfix

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/permutil"
)

func Fix() error {
	u, err := user.Current()
	if err != nil {
		return utils.Errorf("cannot found current system user: %s", err)
	}

	switch runtime.GOOS {
	case "linux":
		// setcap cap_net_raw,cap_net_admin,cap_net_bind_service+eip
		f, err := os.Executable()
		if err != nil {
			return utils.Errorf("cannot locate os.Executable: %v", err)
		}
		return permutil.Sudo(`setcap cap_net_raw,cap_net_admin,cap_net_bind_service+eip ` + strconv.Quote(f))
	case "windows":
		return utils.Error("in windows, u should just start yakit or yak.exe as administrator, or set acl for wpcap.dll")
	case "darwin":
		var haveAccessBPF bool
		var containsUser bool

		groups, _ := exec.Command("dscl", ".", "list", "/Groups").CombinedOutput()
		for _, i := range utils.ParseStringToLines(string(groups)) {
			if strings.TrimSpace(i) == "access_bpf" {
				haveAccessBPF = true
				break
			}
		}

		/*
			dscl . delete /Groups/access_bpf
		*/

		if !haveAccessBPF {
			// dscl . create /Groups/access_bpf gid 101 && chgrp access_bpf /dev/bpf* && chmod g+rw /dev/bpf* && dscl . append /Groups/access_bpf GroupMembership <user>
			cmd := `dscl . create /Groups/access_bpf gid 101 &&` +
				` chgrp access_bpf /dev/bpf* && chmod g+rw /dev/bpf* &&` +
				` dscl . append /Groups/access_bpf GroupMembership ` + strconv.Quote(u.Username)
			err := permutil.Sudo(cmd, permutil.WithVerbose("create access_bpf(101) for fix pcap permission for user"))
			if err != nil {
				return err
			}
			return nil
		}

		log.Infof("checking for user: %v", strconv.Quote(u.Username))
		// dscl . -read /Groups/access_bpf GroupMembership
		raw, _ := exec.Command(`dscl`, `.`, `-read`, `/Groups/access_bpf`, "GroupMembership").CombinedOutput()
		results := strings.TrimSpace(string(raw))

		if strings.HasPrefix(results, "No such key:") {
			log.Info("access_bpf contains no users")
		} else if strings.HasPrefix(results, "GroupMembership:") {
			usersRaw := strings.TrimSpace(results[len("GroupMembership:"):])
			if strings.Contains(usersRaw, "\n") {
				for _, uName := range utils.ParseStringToLines(usersRaw) {
					if u.Username == strings.TrimSpace(uName) {
						containsUser = true
						break
					}
				}
			} else {
				for _, uName := range utils.PrettifyListFromStringSplited(usersRaw, " ") {
					if u.Username == strings.TrimSpace(uName) {
						containsUser = true
						break
					}
				}
			}
		}

		if !containsUser {
			//  uninstall for dscl . delete /Groups/access_bpf GroupMembership <user>
			//  check groupmember: dscacheutil -q group -a name access_bpf
			appendUserToGroupCmd := "dscl . append /Groups/access_bpf GroupMembership " + strconv.Quote(u.Username) + " && chmod g+rw /dev/bpf*"
			_ = appendUserToGroupCmd
			err := permutil.Sudo(appendUserToGroupCmd, permutil.WithVerbose(fmt.Sprintf("add group(access_bpf) for %v", strconv.Quote(u.Username))))
			if err != nil {
				return err
			}
			return nil
		}

		if containsUser {
			log.Infof("access_bpf contains user: %v", strconv.Quote(u.Username))
			return nil
		}

		return utils.Errorf("cannot found group access_bpf: %s", err)
	}
	return nil
}
