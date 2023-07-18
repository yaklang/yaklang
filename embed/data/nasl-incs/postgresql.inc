###############################################################################
# OpenVAS Include File
# $Id: postgresql.inc 13208 2019-01-22 07:11:16Z ckuersteiner $
#
# Functions for PostgreSQL
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
# Christian Kuersteiner <christian.kuersteiner@greenbone.net>
#
# Copyright:
# Copyright (c) 2019 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
#
# $Revision: 13208 $
#
###############################################################################

# @brief Tries to login to the PostgreSQL server without SSL
#
# @param socket       The existing socket to the server
# @param user         Username
# @param password     The password
# @param db           Database name to connect to (optional), default: "postgres"
#
# @return TRUE if login was successful
#         FALSE otherwise
#
# Note: Mainly taken from check_login() in postgresql_detect.nasl
#
# TODO: - Support SSL
#
function postgresql_login(socket, user, password, db) {
  local_var socket, user, password, db, h, null, req, res, len, x, typ, salt, userpass, pass, passlen, code;

  if (!socket) {
    set_kb_item(name: "nvt_debug_empty/" + get_script_oid(),
                value: get_script_oid() + "#-#socket#-#postgresql_login" );
    exit(0);
  }

  h = raw_string((0x03 >> 8) & 0xFF, 0x03 & 0xFF,(0x00 >> 8) & 0xFF, 0x00 & 0xFF);
  null = raw_string(0);

  if (!db)
    db = "postgres";

  req = string(h,
               "user", null, user,
               null,
               "database", null, db,
               null,
               "client_encoding", null, "UNICODE",
               null,
               "DateStyle", null, "ISO",
               null, null);

  len = strlen(req) + 4;
  req = raw_string((len >> 24 ) & 0xff, (len >> 16 ) & 0xff, (len >>  8 ) & 0xff, (len) & 0xff) + req;

  send(socket: socket, data: req);
  res = recv(socket: socket, length: 1);

  if (isnull(res) || res[0] != "R")
    return FALSE;

  res += recv(socket: socket, length: 4);
  if (strlen(res) < 5)
    return FALSE;

  x = substr(res, 1, 4);

  len = ord(x[0]) << 24 | ord(x[1]) << 16 | ord(x[2]) << 8 | ord(x[3]);
  res += recv(socket: socket, length: len);

  if (strlen(res) < len || strlen(res) < 8)
    return FALSE;

  typ = substr(res, strlen(res)-6, strlen(res)-5);
  typ = ord(typ[1]);

  if (typ != 5)
    return FALSE;

  salt = substr(res, strlen(res)-4);
  userpass = hexstr(MD5(password + user));
  pass = 'md5' + hexstr(MD5(userpass + salt));

  passlen = strlen(pass) + 5;

  req = string(raw_string(0x70), raw_string((passlen >> 24 ) & 0xff, (passlen >> 16 ) & 0xff,
               (passlen >>  8) & 0xff, (passlen) & 0xff), pass, raw_string(0));
  send(socket: socket, data: req);

  res = recv(socket: socket, length: 1);

  if(isnull(res) || res[0] != "R")
    return FALSE;

  res += recv(socket: socket, length: 8);

  if (strlen(res) < 8)
    return FALSE;

  code = substr(res, 5, strlen(res));

  # Authentication type: Success
  if (res[0] == "R" && hexstr(code) == "00000000")
    return TRUE;
  else
    return FALSE;
}
