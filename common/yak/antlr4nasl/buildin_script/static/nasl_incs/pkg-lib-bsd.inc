###############################################################################
# OpenVAS Vulnerability Test include file
# $Id: pkg-lib-bsd.inc 10242 2018-06-19 05:45:15Z cfischer $
#
# Check if freebsd packages/patehlevels are up to date
#
# Authors:
# Thomas Reinke <reinke@securityspace.com>
#
# Copyright:
# Copyright (c) 2008 E-Soft Inc. http://www.securityspace.com
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License Version 2
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
# to True if portver() is called for an OS version and a package name that
# match the target's configuration. It allows the callers to distinguish
# between non-installed packages and packages installed but not vulnerable.
__pkg_match = FALSE;

# portver is a quick and easy way for us to determine if a particular package
# exists on a freebsd system., and if so, to return the version number of
# that package.
function portver(pkg) {

  pkgs = get_kb_item("ssh/login/freebsdpkg");
  if(!pkgs) return(NULL);
  narrowed = egrep(pattern:"^" + pkg + "-[0-9]", string:pkgs);
  if(!narrowed) return(NULL);

  # Package was installed
  __pkg_match = TRUE;

  list = split(narrowed, sep:'\n', keep:FALSE);
  foreach package (list) {
    pat = string("^",pkg,"-([^ -]+) (.*)$");
    matches = eregmatch(pattern:pat, string:package);
    if(!isnull(matches)) {
      return(matches[1]);
    }
  }
  return(NULL);
}

# Function to verify if a given patch level is outdated, and if so, to
# report that fact.  Relies on the kb to contain bsdrel and patchlevel.
function patchlevelcmp(rel, patchlevel) {

  kbrel = get_kb_item("ssh/login/freebsdrel");
  if(kbrel !=rel) return(0);
  kbpatchlevel = int(get_kb_item("ssh/login/freebsdpatchlevel"));
  if(kbpatchlevel<int(patchlevel)) {
    return(-1);
  } else if(kbpatchlevel>int(patchlevel)) {
    return(1);
  } else {
    return(0);
  }
}