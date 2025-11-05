-- Enable WiFi Hotspot (Internet Sharing) with admin privileges
-- This script will prompt for admin password automatically when needed

try
    -- Enable Internet Sharing via defaults and launchctl
    do shell script "
        defaults write /Library/Preferences/SystemConfiguration/com.apple.nat NAT -dict Enabled -int 1
        launchctl load -w /System/Library/LaunchDaemons/com.apple.NetworkSharing.plist 2>/dev/null || true
    " with administrator privileges
    return "enabled"
on error errMsg
    return "Error: " & errMsg
end try

