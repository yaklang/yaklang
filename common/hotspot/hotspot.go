package hotspot

import (
	"context"
	"os/exec"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	DefaultSSID     = "Yakit-MITM-Wifi"
	DefaultPassword = "123456"
)

// Status represents the hotspot status
type Status struct {
	Enabled bool
	SSID    string
}

// Enable enables the WiFi hotspot. If already enabled, returns current status.
// Returns: status and error
func Enable() (Status, error) {
	ctx := context.Background()

	// Check current status first
	currentStatus, err := GetStatus()
	if err != nil {
		return Status{}, utils.Errorf("failed to check current status: %v", err)
	}

	if currentStatus.Enabled {
		log.Infof("Hotspot is already enabled with SSID: %s", currentStatus.SSID)
		return currentStatus, nil
	}

	// Enable hotspot
	err = enableHotspot(ctx)
	if err != nil {
		return Status{}, err
	}

	log.Infof("Hotspot enabled successfully - SSID: %s", DefaultSSID)
	return Status{
		Enabled: true,
		SSID:    DefaultSSID,
	}, nil
}

// Disable disables the WiFi hotspot
func Disable() error {
	ctx := context.Background()
	return disableHotspot(ctx)
}

// GetStatus returns the current hotspot status
func GetStatus() (Status, error) {
	ctx := context.Background()
	return getHotspotStatus(ctx)
}

// enableHotspot enables the hotspot using AppleScript with admin privileges
func enableHotspot(ctx context.Context) error {
	script := getEnableScript()

	// Execute with admin privileges using osascript
	// osascript will prompt for admin password automatically
	cmd := exec.CommandContext(ctx, "osascript", "-e", script, DefaultSSID, DefaultPassword)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("failed to enable hotspot: %v, output: %s", err, string(output))
	}

	log.Infof("Hotspot enabled: %s", strings.TrimSpace(string(output)))
	return nil
}

// disableHotspot disables the hotspot using AppleScript with admin privileges
func disableHotspot(ctx context.Context) error {
	script := getDisableScript()

	// Execute with admin privileges using osascript
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("failed to disable hotspot: %v, output: %s", err, string(output))
	}

	log.Infof("Hotspot disabled: %s", strings.TrimSpace(string(output)))
	return nil
}

// getHotspotStatus checks if the hotspot is currently enabled
func getHotspotStatus(ctx context.Context) (Status, error) {
	// Check if Internet Sharing service is loaded
	cmd := exec.CommandContext(ctx, "launchctl", "list", "com.apple.InternetSharing")
	output, err := cmd.CombinedOutput()

	// If the service is not in the list, it's disabled
	enabled := err == nil && len(output) > 0

	status := Status{
		Enabled: enabled,
	}

	if enabled {
		status.SSID = DefaultSSID
	}

	return status, nil
}
