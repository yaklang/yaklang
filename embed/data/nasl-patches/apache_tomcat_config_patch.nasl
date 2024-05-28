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

if(description)
{
  script_oid("1.3.6.1.4.1.25623.1.0.116405");
  script_version("2021-10-06T08:38:20+0000");
  script_tag(name:"last_modification", value:"2021-10-06 08:38:20 +0000 (Wed, 06 Oct 2021)");
  script_tag(name:"creation_date", value:"2021-10-06 08:00:00 +0000 (Wed, 06 Oct 2021)");
  script_tag(name:"cvss_base", value:"0.0");
  script_tag(name:"cvss_base_vector", value:"AV:L/AC:H/Au:S/C:N/I:N/A:N");
  script_tag(name:"qod", value:"97");

  script_name("Apache Tomcat: Configuration Defaults");

  script_category(ACT_SETTINGS);
  script_copyright("Copyright (C) 2021 Greenbone Networks GmbH");
  script_family("Compliance");
  script_dependencies("compliance_tests.nasl");
  script_mandatory_keys("Compliance/Launch");

  script_add_preference(name:"Home directory", type:"entry", value:"/usr/share/tomcat9", id:1);

  script_tag(name:"summary", value:"Configure the Apache Tomcat directory for policy compliance tests.");

  exit(0);
}

prefix = script_get_preference("Home directory", id:1);
if(prefix != "")
  set_kb_item(name:"Policy/ApacheTomcat/home", value:prefix);
else
  set_kb_item(name:"Policy/ApacheTomcat/home", value:"/usr/share/tomcat9");


exit(0);