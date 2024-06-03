# Copyright (C) 2020 Greenbone Networks GmbH
# Some functions might have been moved from misc_func.inc into this
# include and are Copyright (C) by the respective right holder(s).
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

# Enables debugging output for a few functions below
global_var _string_hex_func_debug;
_string_hex_func_debug = 0;

# hex2raw was written by Renaud?
# nb: The previous "url_hex2raw" from url_func.inc (which was a copy of this function)
# had the following comment:
# function taken from amap.nasl which is GPL
function hex2raw( s ) {

  local_var s;
  local_var l, i, j, ret;

  if( isnull( s ) )
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#s#-#hex2raw" );

  s = chomp( s ); # remove trailing blanks, CR, LF...
  l = strlen( s );
  if( l % 2 ) {
    if( _string_hex_func_debug ) display( "hex2raw: odd string: ", s, "\n" );
    l--;
  }
  s = tolower( s );
  for( i = 0; i < l; i += 2 ) {
    if( ord( s[i] ) >= ord( "0" ) && ord( s[i] ) <= ord( "9" ) )
      j = int( s[i] );
    else
      j = int( ( ord( s[i] ) - ord( "a" ) ) + 10 );

    j *= 16;
    if( ord( s[i + 1] ) >= ord( "0" ) && ord( s[ i + 1] ) <= ord( "9" ) )
      j += int( s[i+1] );
    else
      j += int( ( ord( s[ i + 1 ] ) - ord( "a" ) ) + 10 );
    ret += raw_string( j );
  }
  return ret;
}
