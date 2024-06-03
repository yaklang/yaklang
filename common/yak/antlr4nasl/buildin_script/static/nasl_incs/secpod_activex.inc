###############################################################################
# OpenVAS Vulnerability Test
# $Id: secpod_activex.inc 12369 2018-11-16 07:00:32Z cfischer $
#
# ActiveX Control functions
#
# Authors:
# Chandrashekhar B <bchandra@secpod.com>
#
# Copyright:
# Copyright (c) 2009 SecPod, http://www.secpod.com
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
###############################################################################

# Check if killbit is set for the given ActiveX control.
# clsid - CLSID for the given ActiveX control

## Updated secpod_activex.inc to check for 67109888
## i.e 0x400 == 1024 and 0x4000400 == 67109888
## In the MS13-090 workaround also they have to set it as dword:04000400 == 67109888
## After applying the patch killbit registry value is updating to 0x4000400 == 67109888
## Hence added int(killBit) == 67109888 check condition

function is_killbit_set(clsid)
{
  if(clsid)
  {
    if(registry_key_exists(key:"SOFTWARE\Classes\CLSID\" + clsid))
    {
      activeKey = "SOFTWARE\Microsoft\Internet Explorer\" +
                  "ActiveX Compatibility\" + clsid;
      killBit = registry_get_dword(key:activeKey, item:"Compatibility Flags");
      if(killBit && (int(killBit) == 1024 || int(killBit) == 67109888)){
        return 1;
      }
      return 0;
    }
  }
  return -1;
}
