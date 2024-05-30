##############################################################################
# OpenVAS Vulnerability Test
# $Id$
# Description: Internet Explorer Hotfix Supersede check function
#
# Authors:
# Chandan S <schandan@secpod.com>
#
# Copyright:
# Copyright (C) 2008 SecPod, http://www.secpod.com
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
##############################################################################


msBulletin = make_list("MS02-005", "MS02-015", "MS02-023", "MS02-047", "MS02-066", "MS02-068", "MS03-004", "MS03-015", "MS03-020", "MS03-032", "MS03-040", "MS03-048", "MS04-004", "MS04-025", "MS04-038", "MS04-040", "MS05-014", "MS05-020", "MS05-025", "MS05-038", "MS05-052", "MS05-054", "MS06-004", "MS06-013", "MS06-021", "MS06-042", "MS06-067", "MS06-072", "MS07-016", "MS07-027", "MS07-033", "MS07-045", "MS07-057", "MS07-069", "MS08-010", "MS08-024", "MS08-031", "MS08-045", "MS08-058", "MS08-073");

msHotfix = make_list("316059", "319182", "321232", "323759", "328970", "324929", "810847", "813489", "818529", "822925", "828750", "824145", "832894", "867801", "834707", "889293", "867282", "890923", "883939", "896727", "896688", "905915", "910620", "912812", "916281", "918899", "922760", "925454", "928090", "931768", "933566", "937143", "939653", "942615", "944533", "947864", "950759", "953838", "956390", "958215");


function ie_latest_hotfix_update(bulletin)
{
  found = FALSE;

  for(i=0; i>=0; i++)
  {
    if(isnull(msBulletin[i])){
      break;
    }
    else if(bulletin < msBulletin[i])
    {
      tmp = hotfix_missing(name:msHotfix[i]);
      if(tmp == 0)
        found = TRUE;
    }
    if(found){
      break;
    }
  }
  return found;
}
