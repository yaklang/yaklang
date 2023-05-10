###############################################################################
# OpenVAS Vulnerability Test
# $Id: dump.inc 4421 2016-11-04 16:12:14Z cfi $
#
# Some support functions for dump
#
# Authors:
# Tenable Network Security
# An earlier iteration of this file was written by George Dagousset (2002)
#
# Copyright:
# Copyright (C) 2004 Tenable Network Security
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2,
# as published by the Free Software Foundation
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
# $Revision: 4421 $
#
###############################################################################

function line2string( line, linenumber ) {

  local_var tmp, pos, line, linenumber;

  if( isnull( line ) ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#line#-#line2string" );

  line = line * 16;

  tmp = raw_string( ( line >> 24 ) & 0xFF,
                    ( line >> 16 ) & 0xFF,
                    ( line >> 8 ) & 0xFF,
                    ( line ) & 0xFF );

  if( linenumber < 256 ) {
    pos = 3;
  } else if( linenumber < 65536) {
    pos = 2;
  } else if( linenumber < 16777216 ) {
    pos = 1;
  } else {
    pos = 0;
  }

  return string( "0x", toupper( hexstr( substr( tmp, pos, 3 ) ) ), ": " );
}

function isprint( c ) {

  local_var c;

  if( isnull( c ) ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#c#-#isprint" );

  if( ( ord( c ) >= 0x20 ) && ( ord( c ) <= 0x7E) ) {
    return TRUE;
  } else {
   return FALSE;
  }
}


function hexdump( ddata ) {

  local_var tmp, i, j, line, linenumber, len, data, c, ddata;

  if( isnull( ddata ) ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#ddata#-#hexdump" );

  len = strlen( ddata );
  linenumber = len / 16;

  for( i = 0; i <= linenumber; i++ ) {

    line = line2string( line:i, linenumber:len );
    data = "";

    for( j = 0; j < 16; j++ ) {
      if( ( i*16 + j ) < len ) {
        line += string(" ", toupper( hexstr( ddata[i*16+j] ) ) );

        c = ddata[i*16+j];

        if( isprint( c:c ) ) {
          data += c;
        } else {
          data += ".";
        }
      } else {
        line += "   ";
        data += " ";
      }
    }

    tmp += string( line, "    ", data, "\n" );
  }

  return tmp;
}

function dump( dtitle, ddata ) {

  local_var dtitle, ddata;

  if( isnull( ddata ) ) set_kb_item( name: "nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#ddata#-#dump" );

  if( ! isnull( dtitle ) ) display( '---[ ' + dtitle + ' ]---\n' );

  display( hexdump( ddata:ddata ) );
}
