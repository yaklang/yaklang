//go:build !windows

package trafficfixture

import (
	"fmt"
	"os"
)

func preparePrivateWorkspace(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("trafficfixture: private workspace requires a nonsymlink directory: %s", directory)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return err
	}
	return checkPrivateWorkspacePermissions(directory, true)
}

func checkPrivateWorkspacePermissions(name string, root bool) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() || root && !info.IsDir() {
		return fmt.Errorf("trafficfixture: invalid private workspace object: %s", name)
	}
	if info.Mode().Perm()&0077 != 0 || info.Mode().Perm()&0600 != 0600 || info.IsDir() && info.Mode().Perm()&0100 == 0 {
		return fmt.Errorf("trafficfixture: workspace object is not owner-private: %s", name)
	}
	return nil
}
