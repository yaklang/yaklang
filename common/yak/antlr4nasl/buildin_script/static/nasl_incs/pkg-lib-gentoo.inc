###############################################################################
# OpenVAS Include File
# $Id: pkg-lib-gentoo.inc 10242 2018-06-19 05:45:15Z cfischer $
#
# Methods to handle gentoo package lists
#
# Authors:
# Anonymous
#
# Copyright:
# Copyright (C) 2003 Anonymous
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
# $Revision: 10242 $
#
###############################################################################

# This variable is defined at inclusion time. It's declared global and is set
# to True if ispkgvuln() is called for an OS version and a package name that
# match the target's configuration. It allows the callers to distinguish
# between non-installed packages and packages installed but not vulnerable.
__pkg_match = FALSE;

# gentoocheck is intended to provide a quick way to verify that
# a given gentoo pkg, if it is installed, is up to date.
# ispkgvuln returns 0 if it is not installed or is installed
# but up to date, and returns 1 if it is installed and vulnerable

function ispkgvuln(pkg, unaffected, vulnerable) {

  local_var kbrls, pkgs, narrowed, list, vver, vvercomp, rc, res, sub, report;

  # Check that we have the data for this release.
  kbrls = get_kb_item("ssh/login/release");
  if(kbrls!="GENTOO") {
    return NULL;
  }
  pkgs = get_kb_item("ssh/login/pkg");
  if(!pkgs) return NULL;
  narrowed = egrep(pattern:"^" + pkg + "-[0-9]", string:pkgs);
  if(!narrowed) return NULL;
  list = split(narrowed, sep:'\n', keep:FALSE);

  # Package installed.
  __pkg_match = TRUE;

  foreach package (list) {
    # First check if anything is matches the vulnerable versions
    foreach vver (vulnerable) {
      vvercomp = split(vver, sep:' ', keep:FALSE);
      rc = revcomp(a:package, b:pkg + "-" + vvercomp[1]);
      res = 0;
      if(vvercomp[0]=="lt" && rc<0) res = 1;
      if(vvercomp[0]=="le" && rc<=0) res = 1;
      if(vvercomp[0]=="gt" && rc>0) res = 1;
      if(vvercomp[0]=="ge" && rc>=0) res = 1;
      if(vvercomp[0]=="eq" && rc==0) res = 1;
    }

    # If no vulnerability versions match, exit now.
    if(res==0) {
      return NULL;
    }

    # If we get here, we got a match. Check now to see if it
    # matches any of our unaffected versions.
    foreach vver (unaffected) {
      vvercomp = split(vver, sep:' ', keep:FALSE);
      rc = revcomp(a:package, b:pkg + "-" + vvercomp[1]);
      if(vvercomp[0]=="lt" && rc<0) res = 0;
      if(vvercomp[0]=="le" && rc<=0) res = 0;
      if(vvercomp[0]=="gt" && rc>0) res = 0;
      if(vvercomp[0]=="ge" && rc>=0) res = 0;
      if(vvercomp[0]=="eq" && rc==0) res = 0;
      if((vvercomp[0]=="rge" && rc>=0) || (vvercomp[0]=="rgt" && rc>0)) {
        sub = eregmatch(pattern:"(.*-r)[0-9]+$", string:vvercomp[1]);
        if(!sub) sub = vvercomp[1];
        if(sub >< package) res = 0;
      }
    }
    if(res==1) {
      report = 'Package ' + package + ' is installed which is known to be vulnerable.\n';
      return report;
    }
  }
  return NULL;
}