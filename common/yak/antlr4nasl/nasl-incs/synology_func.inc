# Copyright (C) 2022 Greenbone Networks GmbH
# Some text descriptions might be excerpted from (a) referenced
# source(s), and are Copyright (C) by the respective right holder(s).
#
# SPDX-License-Identifier: GPL-2.0-or-later
#
# This program is free software; you can redistribute it and/or
# modify it under the terms of the GNU General Public License
# as published by the Free Software Foundation; either version 2
# of the License, or (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.

# @brief Takes the build number argument and returns a full version based on the release notes from
#        https://www.synology.com/en-us/releaseNote/SRM
#
# @param buildNumber the build number of the release
#
# @return A string containing the full version
#
# @note This mapping is based on intervals of builds numbers. Rather than using the greatest build number for a certain version,
#       it is using the smallest build number of the next version, as exclusive upper limit.
#
function synology_srm_build_number_to_full_version( buildNumber ) {

  local_var buildNumber, int_ver;

  # nb: No "!buildNumber" because there might be also a build number of "0" (if that can ever happen)
  if( isnull( buildNumber ) )
    return NULL;

  int_ver = int( buildNumber );
  if( int_ver < 6007 )
    return "1.0-" + buildNumber;
  if( int_ver < 6022 )
    return "1.0.1-" + buildNumber;
  if( int_ver < 6030 )
    return "1.0.2-" + buildNumber;
  if( int_ver < 6338 )
    return "1.0.3-" + buildNumber;
  if( int_ver < 6414 )
    return "1.1-" + buildNumber;
  if( int_ver < 6425 )
    return "1.1.1-" + buildNumber;
  if( int_ver < 6447 )
    return "1.1.2-" + buildNumber;
  if( int_ver < 6509 )
    return "1.1.3-" + buildNumber;
  if( int_ver < 6542 )
    return "1.1.4-" + buildNumber;
  if( int_ver < 6931 )
    return "1.1.5-" + buildNumber;
  if( int_ver < 6941 )
    return "1.1.6-" + buildNumber;
  if( int_ver < 7742 )
    return "1.1.7-" + buildNumber;
  if( int_ver < 7779 )
    return "1.2-" + buildNumber;
  if( int_ver < 7915 )
    return "1.2.1-" + buildNumber;
  if( int_ver < 8017 )
    return "1.2.2-" + buildNumber;
  if( int_ver < 8081 )
    return "1.2.3-" + buildNumber;
  if( int_ver < 8225 )
    return "1.2.4-" + buildNumber;
  if( int_ver < 9193 )
    return "1.2.5-" + buildNumber;
  if( int_ver < 9346 )
    return "1.3-" + buildNumber;

  return "1.3.1-" + buildNumber;
}

# @brief Checks if the Synology device model is a router or not
#
# @param model the Synology device model
#
# @return TRUE if it is a Synology Router device, FALSE otherwise
#
# @note This is needed in detections that do not differentiate between DSM and SRM (mDNS, UPnP)
#
function check_is_synology_router( model ) {

  local_var model;

  if( "MR2200ac" >< model || "WRX560" >< model || model =~ "^RT" )
    return TRUE;

  return FALSE;
}
