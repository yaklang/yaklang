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

# @brief Returns an array containing the URL for a Canon printer detection
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
function get_canon_detect_urls() {

  local_var canon_detect_urls;

  canon_detect_urls = make_array();

  canon_detect_urls["/index.html"] = "nowrap>Canon ([A-Z0-9]+)"; # nowrap>Canon MX920 series</td>
  canon_detect_urls["/index.html#--avoid-dup1--#"] = 'id="EID" value=.*;mdl=([A-Z0-9]+)'; # id="EID" value="http://rs.ciggws.net/rd.cgi?FNC=RUI_EID2&amp;RES=3&amp;DEV=MB5100+series&amp;CNM_SEP=0&amp;mdl=MB5100+series
  canon_detect_urls["/"] = "<title>User Authentication : ([^<:]+)[:<]"; # <title>User Authentication : iR-ADV C3530 : iR-ADV C3530</title>
  canon_detect_urls["/#--avoid-dup1--#"] = "<title>Remote UI: Login: ([^&<:]+)[:<]"; # <title>Remote UI: Login: LBP6030w/6018w: LBP6030w/6018w</title>
  canon_detect_urls["/#--avoid-dup2--#"] = 'id="deviceName">(MF[^&/< ]+)[&/< ]'; # <span id="deviceName">MF8200C&nbsp;Series / MF8200C&nbsp;Series / </span>
  canon_detect_urls["/_top.html"] = ">Product Name:</font></td>[^<]+<td nowrap>([^<]+)<"; # <td nowrap><font size="1">Product Name:</font></td> <td nowrap>imageRUNNER2530</td>
  canon_detect_urls["/netconf.html"] = "<title>Canon ([^ ]+)"; # <title>Canon iX6800 series Network Configuration
  canon_detect_urls["/t_welcom.cgi?page=Language_name&lang=1"] = ">Product Name:</th>[^>]+>([^< ]+)[^<]+<"; # <th>Product Name:</th> <td>MF4500 Series</td>

  return canon_detect_urls;
}
