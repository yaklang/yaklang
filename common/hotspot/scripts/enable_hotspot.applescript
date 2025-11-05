-- Enable WiFi Hotspot (Internet Sharing) with admin privileges
-- This script will prompt for admin password automatically when needed

-- Use sudo to enable Internet Sharing
-- This is more reliable than UI automation
try
    do shell script "sudo launchctl load -w /System/Library/LaunchDaemons/com.apple.InternetSharing.plist" with administrator privileges
    return "enabled"
on error errMsg
    return "Error: " & errMsg
end try

