###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id$
#
# WMI Client Functions for Process and Service
#
# Authors:
# Chandan S <schandan@secpod.com>
#
# Copyright:
# Copyright (c) 2009 Greenbone Networks GmbH, http://www.greenbone.net
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301 USA.
###############################################################################


##############################################################################
# CLASS: Win32_Service Functions
#
# - wmi_svc_prop  - Returns List of Service with Properties.
# - wmi_svc       - Returns List of Service Names.
# - wmi_svc_state - Returns State of Service, Ex, Stopped/Running.
# - wmi_svc_path  - Returns running service installed path.
#
# If svcName = "AppMgmt", returns details of the service.
#
##############################################################################

function wmi_svc_prop(handle, svcName)
{
  if(!svcName){
    query = "Select * from Win32_Service";
  }
  else {
    query = 'Select * from Win32_Service Where Name = ' +
             raw_string(0x22) + svcName + raw_string(0x22);
  }

  svcList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< svcList) || !svcList){
    return(0);
  }

  return wmi_misc_split_res(List:svcList);
}

function wmi_svc(handle, svcName)
{
  if(!svcName){
    query = "Select Caption from Win32_Service";
  }
  else {
    query = 'Select Caption from Win32_Service Where Name = ' +
             raw_string(0x22) + svcName + raw_string(0x22);
  }

  svcList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< svcList) || !svcList){
    return(0);
  }

  svcList = ereg_replace(pattern:"\|", string:svcList, replace:" - ");
  return svcList;
}

function wmi_svc_state(handle, svcName)
{
  if(!svcName){
    query = "Select State from Win32_Service";
  }
  else {
    query = 'Select State from Win32_Service Where Name = ' +
             raw_string(0x22) + svcName + raw_string(0x22);
  }

  svcList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< svcList) || !svcList){
    return(0);
  }

  svcList = ereg_replace(pattern:"\|", string:svcList, replace:" - ");
  return svcList;
}

function wmi_svc_path(handle, svcName)
{
  if(!svcName){
    query = "Select PathName from Win32_Service";
  }
  else {
    query = 'Select PathName from Win32_Service Where Name = ' +
             raw_string(0x22) + svcName + raw_string(0x22);
  }

  svcList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< svcList) || !svcList){
    return(0);
  }

  svcList = ereg_replace(pattern:"[.a-zA-Z0-9_ ]+\|" + '\\"?', string:svcList, replace:"");
  return svcList;
}
