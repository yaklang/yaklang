##############################################################################
# OpenVAS Include File
# $Id: secpod_reg.inc 11699 2018-09-29 11:26:08Z cfischer $
#
# Retrieving Windows Registry Related Queries
#
# Authors:
# Chandan S <schandan@secpod.com>
#
# Updated By: Antu sanadi <santu@secpod.com> on 2010-08-20
#  - Updated the code to support Windows Vista service packs and Hotfixes.
#  - Updated the code to support Windows 7 service packs and hotfixes.
#  - Updated the code to support Windows Server 2008 service packs and hotfixes
#  - Updated the code to support Windows 8 and 2012 service packs and hotfixes on 2013-04-08
#  - Updated the code to support Windows 8.1 service packs and hotfixes on 2013-11-12
#  - Updated the code to support Windows 10 service packs and hotfixes on 2016-01-04
#  - Updated the code to support Windows Server 2008 64-bit service packs and hotfixes
#  - Updated the code to support Windows Vista 64-bit service packs and hotfixes
#  - Updated the code to support Windows Server 2016 service packs and hotfixes
#
#
# Updated By: Sooraj KS <kssooraj@secpod.com> on 2012-05-09
#  - Updated the code to support Windows 7 64-bit service packs and hotfixes
#  - Updated the code to support Windows XP 64-bit service packs and hotfixes
#  - Updated the code to support Windows 2003 64-bit service packs and hotfixes
#  - Updated the code to support Windows Server 2008 R2 service packs and hotfixes
#
# Copyright:
# Copyright (C) 2008 SecPod, http://www.secpod.com
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
#
# $Revision: 11699 $
#
##############################################################################

#-----------------------------------------------------------------------------
# hotfix_check_sp() : This function is to check whether the OS is installed
# with the given Service Pack or not.
#
# Return Values:
#  1 : If the OS is NOT installed with the given Service Pack.
#  0 : If the OS is installed with the given Service Pack.
# -1 : If invalid credentials or could not connect to registry.
#-----------------------------------------------------------------------------

function hotfix_check_sp( nt, win2k, xp, xpx64, win2003, win2003x64, winVista, winVistax64,
                          win7, win7x64, win2008, win2008x64, win2008r2, win8, win8x64, win2012,
                          win2012R2, win8_1, win8_1x64, win10, win10x64, win2016, win2019, win2022 ) {

  local_var winVer, winName, arch, SvPk;

  winVer = get_kb_item( "SMB/WindowsVersion" );
  winName = get_kb_item( "SMB/WindowsName" );
  arch = get_kb_item( "SMB/Windows/Arch" );
  if( ! winVer ) return -1;

  if( nt && ( "4.0" >< winVer ) ) {

    SvPk = get_kb_item( "SMB/WinNT4/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < nt ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2k && ( "5.0" >< winVer ) && ( "Microsoft Windows 2000" >< winName ) ) {

    SvPk = get_kb_item( "SMB/Win2K/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2k ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( xp && ( "5.1" >< winVer ) && ( "Microsoft Windows XP" >< winName ) ) {

    SvPk = get_kb_item( "SMB/WinXP/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < xp ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( xpx64 && ( "5.2" >< winVer ) && ( "Microsoft Windows XP" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/WinXPx64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < xpx64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2003 && ( "5.2" >< winVer ) && ( "Microsoft Windows Server 2003" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/Win2003/ServicePack" );

    if( SvPk ) {
      SvPk = int(ereg_replace(string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2003 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2003x64 && ( "5.2" >< winVer ) && ( "Microsoft Windows Server 2003" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2003x64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2003x64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( winVista && ( "6.0" >< winVer ) && ( "Windows Vista" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/WinVista/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < winVista ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( winVistax64 && ( "6.0" >< winVer ) && ( "Windows Vista" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/WinVistax64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < winVistax64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win7 && ( "6.1" >< winVer ) && ( "Windows 7" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/Win7/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win7 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win7x64 && ( "6.1" >< winVer ) && ( "Windows 7" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win7x64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win7x64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2008 && ( "6.0" >< winVer ) && ( "Windows Server (R) 2008" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/Win2008/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2008 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2008x64 && ( "6.0" >< winVer ) && ( "Windows Server (R) 2008" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2008x64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2008x64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2008r2 && ( "6.1" >< winVer ) && ( "Windows Server 2008 R2" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2008R2/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2008r2 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win8 && ( "6.2" >< winVer ) && ( "Windows 8" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/Win8/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win8 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win8x64 && ( "6.2" >< winVer ) && ( "Windows 8" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win8x64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win8x64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2012 && ( "6.2" >< winVer ) && ( "Windows Server 2012" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2012/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2012 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2012R2 && ( "6.3" >< winVer ) && ( "Windows Server 2012 R2" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2012R2/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2012R2 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win8_1 && ( "6.3" >< winVer ) && ( "Windows 8.1" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/Win8.1/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win8_1) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win8_1x64 && ( "6.3" >< winVer ) && ( "Windows 8.1" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win8.1x64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win8_1x64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if(win10 && ( "6.3" >< winVer ) && ( "Windows 10" >< winName ) && ( arch == "x86" ) ) {

    SvPk = get_kb_item( "SMB/Win10/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win10 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win10x64 && ( "6.3" >< winVer ) && ( "Windows 10" >< winName ) && ( arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win10x64/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win10x64 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2016 && ( "6.3" >< winVer ) && ( "Windows Server 2016" >< winName ) && (arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2016/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2016 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2019 && ( "6.3" >< winVer ) && ( "Windows Server 2019" >< winName ) && (arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2019/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2019 ) {
      return 1;
    } else {
      return 0;
    }
  }

  else if( win2022 && ( "6.3" >< winVer ) && ( "Windows Server 2022" >< winName ) && (arch == "x64" ) ) {

    SvPk = get_kb_item( "SMB/Win2022/ServicePack" );

    if( SvPk ) {
      SvPk = int( ereg_replace( string:SvPk, replace:"\1", pattern:".*Service Pack ([0-9]).*" ) );
    } else {
      SvPk = 0;
    }

    if( SvPk < win2022 ) {
      return 1;
    } else {
      return 0;
    }
  }
  return -1;
}

# @brief This function is to check whether the OS is installed with given hotfix or not.
#
# @note  Deprecated and shouldn't be used anymore because hotfixes might get superseded by others
#        and the function might report an missing hotfix which was removed and superseded by such
#        other/newer one.
#
# @param name The KB number to check, e.g. 953230 or KB953230 (The KB will be stripped automatically)
#
# @return 1 If the hotfix is missing, 0 if the hotfix is present or -1 if it wasn't possible to connect
#         to the remote registry or an invalid input was given to the name parameter.
#
function hotfix_missing( name ) {

  local_var name, KB, key;

  if( ! name ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#name#-#hotfix_missing" );
    return -1;
  }

  KB = get_kb_list( "SMB/Registry/HKLM/SOFTWARE/Microsoft/*" );

  if( isnull( KB ) || max_index( make_list( keys( KB ) ) ) == 0 ) {
    return -1;
  }

  if( "KB" >< name ) {
    name -= "KB";
  } else if( "Q" >< name ) {
    name -= "Q";
  } else if( "M" >< name ) {
    name -= "M";
  }

  foreach key( keys( KB ) ) {
    if( ereg( pattern:"SMB/Registry/HKLM/SOFTWARE/Microsoft/(Updates/.*|Windows NT/CurrentVersion/HotFix)/(KB|Q|M)" + name, string:key ) ) {
      return 0;
    } else if( ereg( pattern:"SMB/Registry/HKLM/SOFTWARE/Microsoft/Windows/CurrentVersion/Component Based Servicing/Packages/[P|p]ackage.?[0-9]*.?for.?KB.*" + name, string:key ) ) {
      return 0;
    }
  }
  return 1;
}

function hotfix_check_domain_controler() {

  local_var product_options;

  product_options = get_kb_item( "SMB/Registry/HKLM/SYSTEM/CurrentControlSet/Control/ProductOptions" );

  if( ! product_options )
    return -1;
  else if( "LanmanNT" >< product_options )
    return 1;
  else
    return 0;
}

function hotfix_check_nt_server() {

  local_var product_options;

  product_options = get_kb_item( "SMB/Registry/HKLM/SYSTEM/CurrentControlSet/Control/ProductOptions" );

  if( ! product_options )
    return -1;
  else if( "WinNT" >< product_options )
    return 0;
  else
    return 1;
}

function hotfix_check_exchange_installed() {

  local_var vers;

  vers = get_kb_item( "SMB/Registry/HKLM/SOFTWARE/Microsoft/Exchange/Setup/ServicePackBuild" );
  if( ! vers )
    return NULL;
  else
    return vers;
}

function hotfix_check_iis_installed() {

  local_var w3svc;

  w3svc = get_kb_item( "SMB/Registry/HKLM/SYSTEM/CurrentControlSet/Services/W3SVC/ImagePath" );
  if( ! w3svc )
    return -1;
  else if( "inetinfo" >!< w3svc )
    return 0;
  else
    return 1;
}
