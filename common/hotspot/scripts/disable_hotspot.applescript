-- Disable WiFi Hotspot (Internet Sharing) with admin privileges
-- This script will prompt for admin password automatically when needed

try
    -- Disable Internet Sharing via launchctl and defaults
    do shell script "
        launchctl unload -w /System/Library/LaunchDaemons/com.apple.NetworkSharing.plist 2>/dev/null || true
        defaults write /Library/Preferences/SystemConfiguration/com.apple.nat NAT -dict Enabled -int 0
    " with administrator privileges
    return "disabled"
on error errMsg
    return "Error: " & errMsg
end try

