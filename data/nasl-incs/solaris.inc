###############################################################################
# OpenVAS Include File
# $Id: solaris.inc 12500 2018-11-23 10:11:55Z cfischer $
#
# Methods to handle solaris local security checks
#
# Authors:
# Tenable Network Security
# Renaud Deraison
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
# $Revision: 12500 $
#
###############################################################################

# Global keys to avoid that we're repeatedly asking redis for something we already know
global_var __ssh_solosversion;

# @brief returns the current solaris version string of the target saved within the KB (ssh/login/solosversion).
#
# @return a string containing the current solaris version of the target or an empty string if no version
#         string exists in the KB.
#
# @todo Maybe move into a spearate (new) include if other pkg-lib-*.inc includes needs this as well.
#
function get_ssh_solosversion() {

  local_var solosvers;

  if( ! isnull( __ssh_solosversion ) ) {
    solosvers = string( __ssh_solosversion );
  } else {
    solosvers = string( get_kb_item( "ssh/login/solosversion" ) );
    __ssh_solosversion = solosvers;
  }
  return solosvers;
}

function patch_installed(showrev, patch)
{
  local_var v, p,r,patches,spatches;
  v = split(patch, sep:"-", keep:0);
  patches = egrep(pattern:"^Patch: " + v[0], string:showrev);
  if ( ! patches ) return 0; # No patch
  #
  # there may be more then one patch version be listed, so split up the result
  # and do a foreach check.
  #
  spatches = split(patches, keep:0);
  foreach r (spatches)
    {
      # Get the revision number of the installed patch
      r = ereg_replace(pattern:"Patch: ([0-9]*-[0-9]*) .*", replace:"\1", string:r);
      p = split(r, sep:"-", keep:0);
      # If the installed patch is newer than or equal to what we expect, consider
      # it installed

      if  ( int(p[1]) >= int(v[1]) )
        {
          return 1; # Patch is installed
        }
    }
  return 0; # Patch is missing
}


# example call:
# solaris_check_patch(release:"5.10", arch:"i386", patch:"141015-01", package:"SUNWesu")

#
# solaris_check_patch() returns :
#	-1 if a patch is missing
#	0  if a patch is not installed but not required (ie: other architecture)
#	1 if the patch is installed
#
function solaris_check_patch(release, arch, patch, package, obsoleted_by)
{
 local_var showrev, r, flag, packages, p;

 if ( "_x86" >< release )
	release -= "_x86";

 kb_release = get_ssh_solosversion() - '\n';
 kb_arch = get_kb_item("ssh/login/solhardwaretype") - '\n';

 if (kb_release != release || kb_arch != arch){
    return 0;
 }

 packages = get_kb_item("ssh/login/solpackages");
 showrev = get_kb_item("ssh/login/solpatches");
 if ( ! packages || ! showrev || !release || !patch ) return 0;

 # Look if at least one of the packages installed are affected
 # by this patch
 flag = 0;
 if ( strlen(package) )
 {
  package = split(package, sep:" ", keep:FALSE);
  foreach p (package)
    {
      if ( egrep(pattern:p , string:packages) ) flag ++;
    }
 } else flag = 1;

 # No package is affected  - return
 if ( flag == 0 ) return 0;
 

 if ( patch_installed(patch:patch, showrev:showrev) )
	return 1; # Installed

 if ( obsoleted_by && patch_installed(patch:obsoleted_by, showrev:showrev) )
	return 1; # Installed

 return -1; # Not installed
}
