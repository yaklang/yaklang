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

#
# @brief Takes the release date argument in the format YYYYMMDD and returns
#        the corresponding version for DedeCMS V5.7 SP2
#
# @param release date string in the format YYYYMMDD
#
# @return A string containing the full version for DedeCMS V5.7 SP2
#
# @note This mapping is based on https://www.dedecms.com/download
#       and http://dedecms.uu2018.com/pl/
#
function dedecms_version_5_7_release_date_to_version( rel ) {

  local_var rel;

  if ( ! rel )
    return "unknown";

  if( "20110624" >< rel ) return "5.7.10";
  if( "20110701" >< rel ) return "5.7.11";
  if( "20111015" >< rel ) return "5.7.15";
  if( "20111111" >< rel ) return "5.7.16";
  if( "20111227" >< rel ) return "5.7.17";
  if( "20120430" >< rel ) return "5.7.18";
  if( "20120621" >< rel ) return "5.7.19";
  if( "20121030" >< rel ) return "5.7.20";
  if( "20121122" >< rel ) return "5.7.22";
  if( "20121218" >< rel ) return "5.7.23";
  if( "20130115" >< rel ) return "5.7.24";
  if( "20130121" >< rel ) return "5.7.25";
  if( "20130401" >< rel ) return "5.7.26";
  if( "20130402" >< rel ) return "5.7.27";
  if( "20130422" >< rel ) return "5.7.28";
  if( "20130606" >< rel ) return "5.7.29";
  if( "20130607" >< rel ) return "5.7.30";
  if( "20130715" >< rel ) return "5.7.31";
  if( "20130922" >< rel ) return "5.7.32";
  if( "20140114" >< rel ) return "5.7.33";
  if( "20140114" >< rel ) return "5.7.33";
  if( "20140115" >< rel ) return "5.7.34";
  if( "20140116" >< rel ) return "5.7.35";
  if( "20140225" >< rel ) return "5.7.36";
  if( "20140228" >< rel ) return "5.7.37";
  if( "20140304" >< rel ) return "5.7.38";
  if( "20140305" >< rel ) return "5.7.39";
  if( "20140311" >< rel ) return "5.7.40";
  if( "20140313" >< rel ) return "5.7.41";
  if( "20140415" >< rel ) return "5.7.42";
  if( "20140606" >< rel ) return "5.7.43";
  if( "20140612" >< rel ) return "5.7.44";
  if( "20140623" >< rel ) return "5.7.45";
  if( "20140627" >< rel ) return "5.7.46";
  if( "20140724" >< rel ) return "5.7.47";
  if( "20140725" >< rel ) return "5.7.48";
  if( "20140814" >< rel ) return "5.7.49";
  if( "20150522" >< rel ) return "5.7.50";
  if( "20150618" >< rel ) return "5.7.58";
  if( "20160726" >< rel ) return "5.7.59";
  if( "20160801" >< rel ) return "5.7.60";
  if( "20160811" >< rel ) return "5.7.61";
  if( "20160816" >< rel ) return "5.7.62";
  if( "20160906" >< rel ) return "5.7.63";
  if( "20160928" >< rel ) return "5.7.64";
  if( "20170303" >< rel ) return "5.7.65";
  if( "20170309" >< rel ) return "5.7.66";
  if( "20170315" >< rel ) return "5.7.68";
  if( "20170330" >< rel ) return "5.7.69";
  if( "20170405" >< rel ) return "5.7.70";
  if( "20171228" >< rel ) return "5.7.71";
  if( "20180104" >< rel ) return "5.7.72";
  if( "20180107" >< rel ) return "5.7.73";
  if( "20180109" >< rel ) return "5.7.74";
  if( "20210623" >< rel ) return "5.7.75";
  if( "20210712" >< rel ) return "5.7.76";
  if( "20210719" >< rel ) return "5.7.77";
  if( "20210806" >< rel ) return "5.7.78";
  if( "20210815" >< rel ) return "5.7.79";
  if( "20210829" >< rel ) return "5.7.80";
  if( "20210915" >< rel ) return "5.7.81";
  if( "20210926" >< rel ) return "5.7.82";
  if( "20211022" >< rel ) return "5.7.83";
  if( "20211123" >< rel ) return "5.7.84";
  if( "20211224" >< rel ) return "5.7.85";
  if( "20220112" >< rel ) return "5.7.86";
  if( "20220114" >< rel ) return "5.7.87";
  if( "20220125" >< rel ) return "5.7.88";
  if( "20220218" >< rel ) return "5.7.89";
  if( "20220225" >< rel ) return "5.7.90";
  if( "20220310" >< rel ) return "5.7.91";
  if( "20220325" >< rel ) return "5.7.92";
  if( "20220504" >< rel ) return "5.7.93";
  if( "20220520" >< rel ) return "5.7.94";
  if( "20220612" >< rel ) return "5.7.95";
  if( "20220627" >< rel ) return "5.7.96";
  if( "20220708" >< rel ) return "5.7.97";
  if( "20220803" >< rel ) return "5.7.98";
  if( "20220915" >< rel ) return "5.7.99";
  if( "20220920" >< rel ) return "5.7.100";
  if( "20220930" >< rel ) return "5.7.101";
  if( "20221106" >< rel ) return "5.7.102";

  return "unknown";
}

#
# @brief Takes the release date argument in the format YYYYMMDD and returns
#        the corresponding version for DedeCMS V6
#
# @param release date string in the format YYYYMMDD
#
# @return A string containing the full version for DedeCMS V6
#
# @note This mapping is based on https://github.com/DedeBIZ/DedeV6/tags
#
function dedecms_version_6_release_date_to_version( rel ) {

  local_var rel;

  if ( ! rel )
    return "unknown";

  if( "20201101" >< rel ) return "6.0.0";
  if( "20201103" >< rel ) return "6.0.1";
  if( "20201127" >< rel ) return "6.0.2";
  if( "20210520" >< rel ) return "6.0.3";
  if( "20220315" >< rel ) return "6.1.0";
  if( "20220320" >< rel ) return "6.1.1";
  if( "20220322" >< rel ) return "6.1.2";
  if( "20220325" >< rel ) return "6.1.6";
  if( "20220403" >< rel ) return "6.1.7";
  if( "20220417" >< rel ) return "6.1.8";
  if( "20220504" >< rel ) return "6.1.9";

  return "unknown";
}
