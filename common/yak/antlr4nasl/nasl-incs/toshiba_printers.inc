# Copyright (C) 2019 Greenbone Networks GmbH
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

# @brief Returns an array containing the URL for a Toshiba printer detection
#        as the array key and the regex to extract the type/model as the value.
#
# @note The array key might contain a pattern like e.g. (numbers incrementing
#       for multiple same URLs):
#
#       #--avoid-dup1--#
#
#       to be able to have different array keys for the same URL.
#       Make sure to strip such patterns away with ereg_replace
#       in the VT calling this function.
#
# @return An array as described in the function description.
#
function get_toshiba_detect_urls() {

  local_var toshiba_detect_urls;

  toshiba_detect_urls = make_array();

  toshiba_detect_urls["/TopAccess/Device/Device.htm"]  = ">Copier Model.*>TOSHIBA [^&]+";  # NOWRAP>Copier Model&nbsp</TD><TD CLASS="clsBTableElement">TOSHIBA e-STUDIO352&nbsp</TD>
  toshiba_detect_urls["/Device/Device.html"]           = 'id="DeviceCopierModel"';
  toshiba_detect_urls["/"]                             = '<title>TOSHIBA (e-STUDIO[^<]+)</title>'; # <TITLE>TOSHIBA e-STUDIO306CS</TITLE>

  return toshiba_detect_urls;
}
