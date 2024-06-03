###############################################################################
# OpenVAS Vulnerability Test Library functions
# $Id$
#
# WMI Client Functions for Windows OperatingSystem
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
# CLASS: Win32_OperatingSystem
##############################################################################

function wmi_os_version(handle)
{
  query = 'Select Version from Win32_OperatingSystem';

  osVer = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< osVer) || !osVer){
    return(0);
  }

  osVer = eregmatch(pattern:"[0-9]\.[0-9]", string:osVer);
  if(osVer[0] == NULL){
    return(0);
  }
  return osVer[0];
}

function wmi_os_type(handle)
{
  query = 'Select ProductType from Win32_OperatingSystem';

  osType = wmi_query(wmi_handle:handle, query:query);
  osType= eregmatch(pattern:"[0-9]", string:osType);

  if(("NTSTATUS" >< osType) || !osType){
    return(0);
  }

  if(osType[0] == NULL){
    return(0);
  }
  return osType[0];
}

function wmi_os_sp(handle)
{
  query = 'Select CSDVersion from Win32_OperatingSystem';

  spVer = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< spVer) || !spVer){
    return(0);
  }

  spVer = eregmatch(pattern:"Service Pack [0-9]", string:spVer);
  if(spVer[0] == NULL){
    return(1);  # Return 1 if OS without Service Pack.
  }
  return spVer[0];
}

function wmi_os_hotfix(handle)
{
  query = 'Select HotfixID from Win32_QuickFixEngineering';

  hfList = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< hfList) || !hfList){
    return(0);
  }

  j = 0;
  hfList = split(hfList, "\n", keep:0);
  for(i=1; i<max_index(hfList); i++)
  {
    hotfix = eregmatch(pattern:"^(KB|M|Q)[0-9v]+", string:hfList[i]);
    if(hotfix[0] != NULL)
    {
      hf[j] = hotfix[0];
      j++;
    }
  }
  if(hf != NULL){
    return hf;
  }
  else return(0);
}

function wmi_os_buildnumber(handle)
{
  query = 'Select BuildNumber from Win32_OperatingSystem';

  buildVal = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< buildVal) || !buildVal){
    return(0);
  }

  buildVal = eregmatch(pattern:"[0-9]+", string:buildVal);

  if(buildVal[0] != NULL){
    return buildVal[0];
  }
  else return(0);
}

function wmi_os_windir(handle)
{
  query = 'Select WindowsDirectory from Win32_OperatingSystem';

  winDir =  wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< winDir || !winDir)){
    return(0);
  }

  return winDir;
}

function wmi_os_sysdir(handle)
{
  query = 'Select SystemDirectory from Win32_OperatingSystem';

  sysDir = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< sysDir || !sysDir)){
    return(0);
  }

  return sysDir;
}

function wmi_os_all(handle)
{
  query = 'Select * from Win32_OperatingSystem';

  winAll = wmi_query(wmi_handle:handle, query:query);

  if(("NTSTATUS" >< winAll || !winAll)){
    return(0);
  }

  return wmi_misc_split_res(List:winAll); 
}
