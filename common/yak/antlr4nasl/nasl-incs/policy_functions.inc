# Copyright (C) 2019 Greenbone Networks GmbH
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


# @brief This function is used by basic policy tests. The silence value is set in
# compliance_tests.nasl. If you start the test within a standard policy like
# IT-Grundschutz, the results are silent by default. If you start the basic
# policy test by themselves, they are verbose.
#
# @param text the text to log
#
# @return NULL
#
function policy_logging(text, error) {

  local_var text, error;
  local_var verbose;

  verbose = get_kb_item("Compliance/verbose");
  if(verbose){
    if(error){
      log_message(data:text, port:0, proto:"Policy/Control/Error");
    }else{
      log_message(data:text, port:0, proto:"Policy/Control");
    }
  }
}

# @brief set kb entry for policy control oid
#
# @param oid of the policy control
#
# @return NULL
#
function policy_add_oid() {
  set_kb_item(name:"PolicyOIDs", value:get_script_oid());
}

# @brief set kb entry for policy control compliance status
#
# @param compliant if result is compliant with the policy control default value
#
# @return NULL
#
function policy_set_compliance(compliant) {

  local_var compliant;

  set_kb_item(name:get_script_oid() + "/COMPLIANT", value:compliant);
}

# @brief set kb entry for policy control result
#
# @param result of the policy control
#
# @return NULL
#
function policy_set_kb(val) {

  local_var val;

  # nb: save as string, because value can be "0", which would lead to not set the kb_entry
  set_kb_item(name:get_script_oid() + "/RESULT", value:chomp(string(val)));
}

# @brief set kb entry for policy control result if one result per user (like hku reg queries)
#
# @param id of the user
# @param val result for the user
#
# @return NULL
#
function policy_set_kb_hcu(id, val) {

  local_var id, val;

  # nb: save as string, because value can be "0", which would lead to not set the kb_entry
  set_kb_item(name:get_script_oid() + "/RESULT/" + id, value:chomp(string(val)));
}

# @brief set kb entry for policy control default value (compliance value)
#
# @param default of the policy control
#
# @return NULL
#
function policy_set_dval(dval) {

  local_var dval;

  # nb: save as string, because value can be "0", which would lead to not set the kb_entry
  set_kb_item(name:get_script_oid() + "/DEFAULT", value:chomp(string(dval)));
}

# @brief set kb entry for policy control fixtext
#
# @param fixtext of the policy control
#
# @return NULL
#
function policy_fixtext(fixtext) {

  local_var fixtext;

  fixtext = str_replace(string:fixtext, find:'\n', replace:' ');
  set_kb_item(name:get_script_oid() + "/FIX", value:fixtext);
}

# @brief set kb entry for policy control title
#
# @param title of the policy control
#
# @return NULL
#
function policy_control_name(title) {

  local_var title;

  title = str_replace(string:title, find:'\n', replace:' ');
  set_kb_item(name:get_script_oid() + "/NAME", value:title);
}

# @brief set kb entry for policy control test type and command
#
# @param type what type of test used in Policy Control (RegKey, SSH_Cmd etc.)
#        cmd what command was used to get the result
#
# @return NULL
#
function policy_testtype(type, cmd) {

  local_var type, cmd;

  set_kb_item(name:get_script_oid() + "/TEST_TYPE", value:type);
  set_kb_item(name:get_script_oid() + "/CMD", value:cmd);
}

# @brief deprecated function for logging regkey entries
#
# @param type of regkey (HKU / HKLM / HKCU)
# @param key of regkey
# @param item of regkey
# @param value result found on host
#
# @return NULL
#
function policy_logging_registry(type, key, item, value) {

  local_var type, key, item, value;

  policy_logging(text:'Registry value ' + type + '\\' + key + '!' + item + ' is set to: ' + value);
}

# @brief build report for Policy Controls
#
# @param result what result found on host
# @param default what need to be set to be compliant
# @param compliant compliance status (yes, no, error)
# @param fixtext how to fix non-compliance
# @param type of test (RegKey, SSH_Cmd etc.)
# @param test command to run / regkey etc.
# @param info additional information like comments
#
# @return NULL
#
function policy_reporting(result, default, compliant, fixtext, type, test, info) {

  local_var result, default, compliant, fixtext, type, test, info;
  local_var report;

  if(!result) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#policy_reporting#-#result");
    return;
  }

  if(!default) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#policy_reporting#-#default");
    return;
  }

  if(!compliant) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#policy_reporting#-#compliant");
    return;
  }

  if(!fixtext) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#policy_reporting#-#fixtext");
    return;
  }

  if(!type) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#policy_reporting#-#type");
    return;
  }

  if(!test) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#policy_reporting#-#test");
    return;
  }

  test = str_replace(string:test, find:'\n', replace: " ");
  fixtext = str_replace(string:fixtext, find:'\n', replace: " ");

  report += "Compliant:    " + toupper(compliant) + '\n';
  report += "Actual Value: " + result + '\n';
  report += "Set Point:    " + default + '\n';
  report += "Type of Test: " + type + '\n';
  report += "Test:         " + test + '\n';
  report += "Solution:     " + fixtext;

  if(info){
    report += '\n' + "Notes:        " + info;
  }

  policy_logging(text:report);
}

# @brief query wmi db RSOP_SecuritySettingBoolean for key
#
# @param select what to show from db
# @param keyname which key to search for
#
# @note rsop only delivers results for domain admins
#
# @return wmi query result
#
function rsop_securitysettingsboolean(select, keyname) {

  local_var select, keyname;
  local_var infos, handle, query, res, splitRes, returnValue;

  infos = kb_smb_wmi_connectinfo();
  if(!infos)
    exit(0);

  handle = wmi_connect(host:infos["host"], username:infos["username_wmi_smb"], password:infos["password"], ns:'root\\rsop\\computer');
  if(!handle) {
    policy_logging(text:"WMI Connect to host failed.");
    policy_set_kb(val:"error");
    exit(0);
  }

  query = "SELECT " + select + " FROM RSOP_SecuritySettingBoolean WHERE KeyName = '" + keyname + "' AND precedence = '1'";
  res = wmi_query_rsop(wmi_handle:handle, query:query);
  wmi_close(wmi_handle:handle);

  splitRes = split(res, sep:"|", keep:FALSE);
  if(max_index(splitRes) < 4)
    return('');

  returnValue = splitRes[4];
  return(string(returnValue));
}

# @brief query wmi db RSOP_SecuritySettingNumeric for key
#
# @param select what to show from db
# @param keyname which key to search for
#
# @note rsop only delivers results for domain admins
#
# @return wmi query result
#
function rsop_securitysettingsnumeric(select, keyname) {

  local_var select, keyname;
  local_var infos, handle, query, res, splitRes, returnValue;

  infos = kb_smb_wmi_connectinfo();
  if(!infos)
    exit(0);

  handle = wmi_connect(host:infos["host"], username:infos["username_wmi_smb"], password:infos["password"], ns:'root\\rsop\\computer');
  if(!handle) {
    policy_logging(text:"WMI Connect to host failed.");
    policy_set_kb(val:"error");
    exit(0);
  }

  query = "SELECT " + select + " FROM RSOP_SecuritySettingNumeric WHERE KeyName = '" + keyname + "' AND precedence = '1'";
  res = wmi_query_rsop(wmi_handle:handle, query:query);
  wmi_close(wmi_handle:handle);

  splitRes = split(res, sep:"|", keep:FALSE);
  if(max_index(splitRes) < 4)
    return('');

  returnValue = splitRes[4];
  return(string(returnValue));
}

# @brief query wmi db RSOP_UserPrivilegeRight for specific right
#
# @param select what to show from db
# @param keyname which user right to search for
#
# @note rsop only delivers results for domain admins
#
# @return accounts as string
#
function rsop_userprivilegeright(select, keyname) {

  local_var select, keyname;
  local_var infos, handle, query, res, splitRes, accountList, MaxIndexAccountList, i, returnValue;

  infos = kb_smb_wmi_connectinfo();
  if(!infos)
    exit(0);

  handle = wmi_connect(host:infos["host"], username:infos["username_wmi_smb"], password:infos["password"], ns:'root\\rsop\\computer');
  if(!handle) {
    policy_logging(text:"WMI Connect to host failed.");
    policy_set_kb(val:"error");
    exit(0);
  }

  query = "SELECT " + select + " FROM RSOP_UserPrivilegeRight WHERE UserRight = '" + keyname + "'";
  res = wmi_query_rsop(wmi_handle:handle, query:query);
  wmi_close(wmi_handle:handle);
  if(res == '')
    return "None";

  splitRes = split(res, keep:FALSE);
  accountList = split(splitRes[1], sep:"|", keep:FALSE);
  MaxIndexAccountList = max_index(accountList);
  for(i = 0; i < MaxIndexAccountList - 2; i++) {
    returnValue += accountList[i];
    if( i < MaxIndexAccountList - 3 ) {
      returnValue += ", ";
    }
  }

  return(string(returnValue));
}

# @brief gets the status of a user account on windows hosts
#
# @param select what to return from WMI query
# @param name of the user account
#
# @return the selection from the wmi db
#
function win32_useraccount(select, name) {

  local_var select, name;
  local_var infos, handle, query, res;

  infos = kb_smb_wmi_connectinfo();
  if(!infos)
    exit(0);

  handle = wmi_connect(host:infos["host"], username:infos["username_wmi_smb"], password:infos["password"]);
  if(!handle) {
    policy_logging(text:"WMI Connect to host failed.");
    policy_set_kb(val:"error");
    exit(0);
  }

  query = "SELECT " + select + " FROM Win32_UserAccount WHERE Name = '" + name + "'";
  res = wmi_query(wmi_handle:handle, query:query);
  wmi_close(wmi_handle:handle);

  return(res);
}

# @brief Checks for a package whether it is installed and if so, which version
#
# @param package The package to check the existence / version
# @param maintainer The package maintainer (RPM or DEB)
#
# @return A string with the version or NULL if not installed on host
#
function get_package_version(package) {

  local_var package;
  local_var packages_string, packages_list, pak, packages_split, name, version, line, line_split;

  if(!package) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#package_version#-#package" );
    return;
  }

  if(get_kb_item("ssh/login/rpms")) {
    packages_string = get_kb_item("ssh/login/rpms");
    packages_list = split(packages_string, sep:";", keep:FALSE);
    foreach pak(packages_list) {
      package_split = split(pak, sep:"~", keep:FALSE);
      name = package_split[0];
      version = package_split[1];
      if(name == package) {
        return version;
      }
    }
  } else if(get_kb_item("ssh/login/packages")) {
    packages_string = get_kb_item("ssh/login/packages");
    packages_list = split(packages_string, keep:FALSE);
    foreach line(packages_list) {
      if(line =~ "^ii") {
        line_split = ereg_replace(string:line, pattern:'\\s+', replace:'|');
        packages_info = split(line_split, sep:"|", keep:FALSE);
        name = packages_info[1];
        version = packages_info[2];
        if(name == package) {
          return version;
        }
      }
    }
  }
  return;
}

# @brief Tries to find a GRUB bootloader config file on the host.
#        Searches for grub.conf, grub.cfg or menu.lst in directories
#        /boot/grub/ or /boot/grub2/.
#
# @return Filepath if config file found or NULL if not found
#         (maybe because of denied read permisisons)
#
function get_grub_config_file() {

  local_var grub_config_files, grub_config_dirs, dir, file, config_file;

  grub_config_files = make_list('grub.conf', 'grub.cfg', 'menu.lst');
  grub_config_dirs = make_list('/boot/grub/', '/boot/grub2/');

  foreach dir(grub_config_dirs) {
    foreach file(grub_config_files) {
      config_file = find_file(file_name:file, file_path:dir);
      if(config_file)
        return config_file[0];
    }
  }
  return;
}

# @brief sends a ssh_cmd and filters errors in return values.
#
# @deprecated Please use @ref ssh_cmd from ssh_func.inc and add possible
#             pattern to filter out directly into that function.
#
# @param socket the socket for the ssh command
# @param cmd the bash command
#
# @return return of the command or NULL if file not found or
#         other error occurred
#
function ssh_cmd_without_errors(socket, cmd) {

  local_var socket, cmd;
  local_var ret;

  ret = ssh_cmd(socket:socket, cmd:cmd, return_errors:FALSE);

  if('No such file or directory' >< ret || 'command not found' >< ret)
    return;

  return(ret);
}

# @brief checks the file permissions, gid and uid with stat command
#        on a linux host
#
# @param file the file to check
# @param sock socket for the ssh command
#
# @return an array with keys permissions, gid and uid or NULL if none found.
#
function linux_file_permissions(file, socket) {

  local_var file, socket;
  local_var stat_cmd, stat, return_array, access, gid, uid, permissions;

  if(!socket) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#linux_file_permissions#-#socket");
    return;
  }

  if(!file) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#linux_file_permissions#-#file");
    return;
  }

  stat_cmd = 'stat ' + file;
  stat = ssh_cmd_without_errors(socket:socket, cmd:stat_cmd);
  if(stat && ('permission denied' >!< tolower(stat))) {
    return_array = make_list();
    access = eregmatch(string:stat, pattern:'(Access:.+Uid.+Gid.+\\))');
    gid = eregmatch(string:access[0], pattern:'Gid: \\((.+)\\)');
    uid = eregmatch(string:access[0], pattern:'Uid: \\(([^\\)]*)\\)');
    permissions = eregmatch(string:access[0], pattern:'Access: \\(([0-9]+)');

    return_array['permissions'] = permissions[1];
    return_array['gid'] = str_replace(string:gid[1], find:' ', replace:'');
    return_array['uid'] = str_replace(string:uid[1], find:' ', replace:'');

    return (return_array);
  }
  return;
}

# @brief check if a service is enabled on a Linux host.
#        Uses systemctl is-enabled, chkconfig --list and
#        at last search for service in /etc/rc*.d files
#
# @param service the service to search for
# @param socket socket for the ssh command
#
# @return TRUE if service is enabled, NULL if not
#
function linux_service_is_enabled(service, socket) {

  local_var service, socket;
  local_var systemctl_cmd, systemctl, chkconfig_cmd, chkconfig, ls_grep_cmd, ls_grep, enabled;

  if(!socket) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#linux_file_permissions#-#socket");
    return;
  }

  if(!service) {
    set_kb_item(name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#linux_file_permissions#-#service");
    return;
  }

  systemctl_cmd = 'systemctl is-enabled ' + service;
  systemctl = ssh_cmd_without_errors(socket:socket, cmd:systemctl_cmd);
  if('enabled' >< systemctl) {
    return TRUE;
  }

  chkconfig_cmd = 'chkconfig --list ' + service;
  chkconfig = ssh_cmd_without_errors(socket:socket, cmd:chkconfig_cmd);
  if('2:on' >< chkconfig && '3:on' >< chkconfig && '4:on' >< chkconfig && '5:on' >< chkconfig) {
    return TRUE;
  }

  ls_grep_cmd = 'ls /etc/rc*.d | grep ' + service;
  ls_grep = ssh_cmd_without_errors(socket:socket, cmd:ls_grep_cmd);
  enabled = eregmatch(string:ls_grep, pattern:'[\\^S][^\n\r]*');
  if(enabled)
    return TRUE;

  return;
}

# @brief check if a service is enabled on a Linux host.
#        Uses systemctl is-enabled, chkconfig --list and
#        at last search for service in /etc/rc*.d files
#
# @param service the service to search for
# @param socket socket for the ssh command
#
# @return TRUE if service is enabled, NULL if not
#
function check_permission_denied(value, file) {

  local_var value, file;
  local_var ret;

  if('permission denied' >< tolower(value)) {
    ret = 'No permission to read in file "' + file + '".';
    return(ret);
  }
  return;
}

# @brief Tests if key SMB/WindowsVersion is at least a given Windows Version.
#
# @param min_ver minimal Windows version (as string)
#
# @return TRUE if SMB/WindowsVersion is set and meets at least the given version, FALSE if not
#
function policy_verify_win_ver(min_ver){
  local_var min_ver, cur_ver;

  if(min_ver && min_ver !~ "^[0-9.]+$") {
    set_kb_item(name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#policy_verify_win_ver: wrong syntax in 'min_ver', passed: " + min_ver + ", expected e.g. 6{.2}" );
    return NULL; # To differ between "FALSE" (its not a windows) and "NULL" (we don't know because syntax is wrong)
  }

  cur_ver = get_kb_item("SMB/WindowsVersion");

  if(!cur_ver)
    return FALSE;

  if(cur_ver && cur_ver !~ "^[0-9.]+$") {
    set_kb_item(name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#policy_verify_win_ver: wrong syntax in 'cur_ver', extracted: " + cur_ver + ", expected e.g. 6{.2}" );
    return NULL; # To differ between "FALSE" (its not a windows) and "NULL" (we don't know because syntax is wrong)
  }

  if(!min_ver && cur_ver)
    return TRUE;

  if(version_is_greater_equal(version:cur_ver, test_version:min_ver))
    return TRUE;

  return FALSE;
}

# @brief Returns reporting variables for wrong os
#
# @param target_os OS of host that should be matched (at least)
#
# @return ret array with value, comment and compliant strings
#
function policy_report_wrong_os(target_os){
  local_var target_os, ret;

  ret["value"] = "None";
  ret["comment"] = "Host does not run " + target_os + ".";
  ret["compliant"] = "incomplete";

  return ret;
}

# @brief Returns results variables for exact reg_dword check
#
# @param key Registry key to check
# @param item Registry item to check
# @param type Registry type to check
# @param default set point to check the actual value against
#
# @return ret array with value, comment and compliant strings
#
function policy_match_exact_reg_dword(key, item, type, default){
  local_var key, item, type, default;
  local_var value, comment, compliant, ret;

  value = registry_get_dword(key:key, item:item, type:type);
  value = chomp(value);

  if(value == ""){
    comment = "Registry key not found.";
    compliant = "incomplete";
    value = "None";
  }else{
    if(int(value) == int(default)){
      compliant = "yes";
    }else{
      compliant = "no";
    }
  }

  ret["value"] = value;
  ret["compliant"] = compliant;
  if(comment){
    ret["comment"] = comment;
  }else{
    ret["comment"] = "";
  }

  return ret;
}

# @brief Returns results variables for exact or partial reg_sz check
#
# @param key Registry key to check
# @param item Registry item to check
# @param type Registry type to check
# @param default set point to check the actual value against
#
# @return ret array with value, comment and compliant strings
#
function policy_match_reg_sz(key, item, type, default, partial){
  local_var key, item, type, default, partial;
  local_var value, comment, compliant, ret;

  value = registry_get_sz(key:key, item:item, type:type);
  value = chomp(value);

  if(value == ""){
    comment = "Registry key not found.";
    compliant = "incomplete";
    value = "None";
  }else{
    if(partial && default >< value){
      compliant = "yes";
    }else if(value == default){
      compliant = "yes";
    }else{
      compliant = "no";
    }
  }

  ret["value"] = value;
  ret["compliant"] = compliant;
  if(comment){
    ret["comment"] = comment;
  }else{
    ret["comment"] = "";
  }

  return ret;
}

# @brief Sets the reult keys to the KB
#
# @param type The type of the test
# @param cmd The actual command of the test
# @param default set point for the policy control
# @param solution how to fix non-compliance
# @param title title of the policy control
# @param value the actual value found on the host
# @param compliant compliance status of the policy control
#
function policy_set_kbs(type, cmd, default, solution, title, value, compliant){
  local_var type, cmd, default, solution, title, value, compliant;

  policy_testtype(type:type, cmd:cmd);
  policy_add_oid();
  policy_set_dval(dval:default);
  policy_fixtext(fixtext:solution);
  policy_control_name(title:title);
  policy_set_kb(val:value);
  policy_set_compliance(compliant:compliant);
}

# @brief Checks if SSH login is possible and if host runs Linux OS
#
# @return boolean TRUE if Linux shell and SSH login was possible, FALSE if not
#
function policy_check_linux_ssh_shell(){
  if(!get_kb_item("login/SSH/success") || get_kb_item("ssh/no_linux_shell") ||
      get_kb_item("ssh/restricted_shell")){
    return(FALSE);
  }
  return(TRUE);
}