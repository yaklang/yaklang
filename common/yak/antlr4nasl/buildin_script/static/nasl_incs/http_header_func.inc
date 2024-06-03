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

# @brief Returns an array of common (client-side) HTTP headers having the given value added to each
#        of the header (created internally by @ref http_hdr_get_common_list). This function can be
#        used to e.g. add the very same payload to every known / common header.
#
# @note No ":" header key/value separator needs to be given to the "value" parameter.
#
# @param value            The (mandatory) value/payload to add to the HTTP headers.
# @param exclude_hdr_list A list (optional) of headers which should be excluded like e.g.
#                         exclude_hdr:make_list("User-Agent"). This can be used if e.g. a specific
#                         header is handled separately in the VT calling this function.
#
# @return An array containing the HTTP headers in the array keys and the passed values in the array
#         value of each key, NULL otherwise if the "value" parameter has been left empty or data has
#         been passed to the "exclude_hdr_list" parameter but the data isn't a list.
#
function http_hdr_add_value( value, exclude_hdr_list ) {

  local_var value, exclude_hdr_list;
  local_var hdr_list, ret_array, _hdr;

  if( ! value ) {
    set_kb_item( name:"vt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#value#-#http_hdr_add_value" );
    return;
  }

  if( exclude_hdr_list && ! is_array( exclude_hdr_list ) ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_hdr_add_value(): No list passed in 'exclude_hdr_list' parameter" );
    return;
  }

  hdr_list = http_hdr_get_common_list( exclude_hdr_list:exclude_hdr_list );
  if( ! hdr_list ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_hdr_add_value(): Failed to craft header list via http_hdr_get_common_list() for unknown reasons" );
    return;
  }

  ret_array = make_array();

  foreach _hdr( hdr_list )
    ret_array[_hdr] = value;

  return ret_array;
}

# @brief Returns a list of common (client-side) HTTP headers.
#
# @param exclude_hdr_list A list (optional) of headers which should be excluded like e.g.
#                         exclude_hdr:make_list("User-Agent"). This can be used if e.g. a specific
#                         header is handled separately in the VT calling this function.
#
# @return A list containing common HTTP headers, NULL otherwise if data has been passed to the
#         "exclude_hdr_list" parameter but the data isn't a list.
#
function http_hdr_get_common_list( exclude_hdr_list ) {

  # nb: Keep this function at the bottom of this include as it might get even longer in the future
  # which would make the include hard to read if it starts with a function having many lines.

  local_var exclude_hdr_list;
  local_var tmp_hdr_list, final_hdr_list, _hdr;

  if( exclude_hdr_list && ! is_array( exclude_hdr_list ) ) {
    set_kb_item( name:"vt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#http_hdr_get_common_list(): No list passed in 'exclude_hdr_list' parameter" );
    return;
  }

  tmp_hdr_list = make_list(
    "Accept",
    "Accept-Charset",
    "Accept-Encoding",
    "Accept-Language",
    "Access-Control-Request-Headers",
    "Access-Control-Request-Method",
    "Authorization",
    "Cache-Control",
    "Content-Type",
    "Cookie",
    "Location",
    "Origin",
    "Pragma",
    "Referer",
    "Upgrade-Insecure-Requests",
    "User-Agent",
    "X-Api-Version",
    "X-CSRF-Token",
    "X-Druid-Comment",
    "X-Forwarded-Prefix",
    "X-Forwarded-For",
    "X-Forwarded-Host",
    "X-Forwarded-Server",
    "X-Forwarded-Scheme",
    "X-Origin",
    "X-Original-URL"
  );

  if( exclude_hdr_list ) {
    final_hdr_list = make_list();
    foreach _hdr( tmp_hdr_list ) {
      if( ! in_array( search:_hdr, array:exclude_hdr_list, part_match:FALSE, icase:TRUE ) ) {
        final_hdr_list = make_list( final_hdr_list, _hdr );
      }
    }
  } else {
    final_hdr_list = tmp_hdr_list;
  }

  return final_hdr_list;
}
