###############################################################################
# Version compare for JunOS
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2015 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
###############################################################################

# compare junos space version
function check_js_version( ver, fix )
{
  if( ! ver )
  {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#ver#-#check_js_version" );
    return;
  }

  if( ! fix )
  {
    set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#fix#-#check_js_version" );
    return;
  }

  ver = str_replace( string:ver, find:"-", replace:"." );
  fix = str_replace( string:fix, find:"-", replace:"." );

  if( ! egrep( pattern:'^[0-9]+\\.[0-9]+((B|R|P)[0-9]+(\\.[0-9]+)?)?', string:ver ) ) return;
  if( ! egrep( pattern:'^[0-9]+\\.[0-9]+((B|R|P)[0-9]+(\\.[0-9]+)?)?', string:fix ) ) return;

  if( ver == fix ) return;

  tver = ereg_replace( pattern:"(B|P|R)([0-9]+)", replace:".\2", string: ver );
  tfix = ereg_replace( pattern:"(B|P|R)([0-9]+)", replace:".\2", string: fix );

  if( tver == tfix )
  {
    # http://developer.juniper.net/shared/jdn/html/browser-help-13.3/com.juniper.junos_space.sdk.help/html/guides/appdevguide/createappimg.html
    # Release Type
    # The type of release for the application. The release type can be one of B1, B2, B3, B4; or R1, R2, R3, R4;
    # or P1, P2, P3, P4 and P5 where:
    #
    # B specifies a Beta Release
    # R specifies a Customer Release
    # P specifies a Patch Release
    #
    # IMHO this means that if both versions are equal: P > R > B

    if( "B" >< ver && ( "R" >< fix || "P" >< fix ) ) return TRUE;
    if( "R" >< ver && "P" >< fix ) return TRUE;

    return;
  }

  v_build = 0;
  f_build = 0;

  ver_array = split( tver, sep:".", keep:FALSE );

  v_major = ver_array[0];
  v_minor = ver_array[1];
  v_rev   = ver_array[2];
  if( ! isnull( ver_array[3] ) ) v_build = ver_array[3];


  fix_array = split( tfix, sep:".", keep:FALSE );

  f_major = fix_array[0];
  f_minor = fix_array[1];
  f_rev   = fix_array[2];
  if( ! isnull( fix_array[3] ) ) f_build = fix_array[3];

  if( v_major < f_major ) return TRUE;
  if( v_major > f_major ) return;

  if( v_minor < f_minor )  return TRUE;
  if( v_minor > f_minor )  return;

  if( v_rev < f_rev ) return TRUE;
  if( v_rev > f_rev ) return;

  if( v_build < f_build ) return TRUE;
  if( v_build > f_build ) return;

  return;

}
