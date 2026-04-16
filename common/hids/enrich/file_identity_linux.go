//go:build hids && linux

package enrich

import (
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"
)

type FileIdentity struct {
	IsDir bool
	Mode  string
	UID   string
	GID   string
	Owner string
	Group string
}

var fileOwnerCache sync.Map
var fileGroupCache sync.Map

func SnapshotFileIdentity(path string) (FileIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return FileIdentity{}, err
	}
	return FileIdentityFromFileInfo(info), nil
}

func FileIdentityFromFileInfo(fileInfo os.FileInfo) FileIdentity {
	identity := FileIdentity{}
	if fileInfo == nil {
		return identity
	}

	identity.IsDir = fileInfo.IsDir()
	identity.Mode = fileInfo.Mode().String()

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return identity
	}

	identity.UID = strconv.FormatUint(uint64(stat.Uid), 10)
	identity.GID = strconv.FormatUint(uint64(stat.Gid), 10)
	identity.Owner = cachedUserName(identity.UID)
	identity.Group = cachedGroupName(identity.GID)
	return identity
}

func cachedUserName(uid string) string {
	if uid == "" {
		return ""
	}
	if value, ok := fileOwnerCache.Load(uid); ok {
		return value.(string)
	}
	name := lookupUserName(uid)
	fileOwnerCache.Store(uid, name)
	return name
}

func cachedGroupName(gid string) string {
	if gid == "" {
		return ""
	}
	if value, ok := fileGroupCache.Load(gid); ok {
		return value.(string)
	}
	name := lookupGroupName(gid)
	fileGroupCache.Store(gid, name)
	return name
}

func lookupUserName(uid string) string {
	entry, err := user.LookupId(uid)
	if err != nil {
		return ""
	}
	return entry.Username
}

func lookupGroupName(gid string) string {
	entry, err := user.LookupGroupId(gid)
	if err != nil {
		return ""
	}
	return entry.Name
}
