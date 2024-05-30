# Copyright (C) 2021 Greenbone Networks GmbH
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

# @brief Returns an array containing the URL for a Epson printer detection
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
function get_epson_detect_urls() {

  local_var epson_detect_urls;

  epson_detect_urls = make_array();

  epson_detect_urls["/PRESENTATION/HTML/TOP/INDEX.HTML"]  = "<title>([A-Z0-9-]+)( Series)?</title>"; # <title>L4160 Series</title>
  epson_detect_urls["/iPrinterHome.cgi"]                  = "Printer Model Name</td>[^>]+>\s*([^>]+)<"; # Printer Model Name</td> <td height="16">AL-MX300</td></tr>
  epson_detect_urls["/PRESENTATION/ADVANCED/FORCE_PASSWORD/TOP?accsessmethod=0"]  = '<title>([A-Z0-9-]+) Series</title>'; # <title>ET-16650 Series</title>
  epson_detect_urls["/Istatus.htm"]                       = "Model Name</B></TD>\[^>]+>Epson ([^&<]+)"; # Model Name</B></TD> <TD width="60%">Epson Stylus Pro 9890&nbsp;</TD>

  return epson_detect_urls;
}
