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

# @brief Returns a list of known VMware Spring Framework components.
#
# @return A list of known VMware Spring Framework components.
#
function spring_framework_comp_list() {

  local_var comp_list;

  comp_list = make_list(
    # The "org.springframework.*" ones from https://github.com/spring-projects/spring-framework/tree/3.0.x
    # but without the "org.springframework" and the "." in the component name replaced by "-".
    # These have been cross-verified against the ones from https://packages.debian.org/search?keywords=libspring
    "aop",
    "asm",
    "aspects",
    "beans",
    "context-support",
    "context",
    "core",
    "expression",
    "instrument-tomcat",
    "instrument",
    "integration-tests",
    "jdbc",
    "jms",
    "orm",
    "oxm",
    "spring-library",
    "spring-parent",
    "test",
    "transaction",
    "web-portlet",
    "web-servlet",
    "web-struts",
    "web",
    # And the spring-* ones from https://github.com/spring-projects/spring-framework/tree/5.3.x
    # but without the "spring-" prefix. Note that a few are shared across both lists.
    "aop",
    "aspects",
    "beans",
    "context-indexer",
    "context-support",
    "context",
    "core",
    "expression",
    "instrument",
    "jcl",
    "jdbc",
    "jms",
    "messaging",
    "orm",
    "oxm",
    "r2dbc",
    "test",
    "tx",
    "web",
    "webflux",
    "webmvc",
    "websocket" );

  comp_list = make_list_unique( comp_list );

  return comp_list;
}

# @brief Returns a string containing a regex "(comp1|comp2|...)" to match all known VMware Struts
#        Framework components (list internally maintained in @ref spring_framework_comp_list).
#
# @return A string containing a regex "(comp1|comp2|...)" to match all known VMware Struts Framework
#         components.
#
function spring_framework_comp_regex() {

  local_var comps, i, comp, pattern;

  comps = spring_framework_comp_list();
  i = 0;

  foreach comp( comps ) {
    i++;
    if( i == 1 )
      pattern = "(" + comp;
    else
      pattern += "|" + comp;
  }

  pattern += ")";

  return pattern;
}
