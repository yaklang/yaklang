###############################################################################
# OpenVAS Include File
# $Id: gvr_apps_auth_func.inc 13658 2019-02-14 08:16:14Z cfischer $
#
# Supporting functions for TYPO3 Detection
#
# Authors:
# Shashi Kiran N <nskiran@secpod.com>
#
# Copyright:
# Copyright (C) 2014 Greenbone Networks GmbH, http://www.greenbone.net
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
#
# $Revision: 13658 $
#
###############################################################################

#To get TYPO3 session cookie
#TYPO3 install path, port and host need to be sent as parameter
function get_typo3_login_cookie(cinstall, tport, chost)
{
  local_var cinstall, tport, chost, url, treq, tres, username, password, challenge;
  local_var payload, tcookie, PHPSESSID, cCookie, req, buf, rcookie;

  url = cinstall + "/typo3/index.php";
  treq = http_get(item:string(url), port:tport);
  tres = http_keepalive_send_recv(port:tport, data:treq, bodyonly:FALSE);

  username = urlencode(str:get_kb_item("http/login"));
  password = urlencode(str:get_kb_item("http/password"));

  challenge = eregmatch(pattern:'name="challenge" value="([a-z0-9]+)"' , string:tres);

  if(challenge)
  {
    password = hexstr(MD5(password));
    userident = hexstr(MD5(username + ":" + password + ":" + challenge[1]));
    payload = "login_status=login&username=" + username + "&p_field=&commandLI=Log+In&" +
              "userident=" + userident + "&challenge=" + challenge[1] + "&redirect_url=" +
              "alt_main.php&loginRefresh=&interface=backend";

    tcookie = eregmatch(pattern:"(be_typo_user=[a-z0-9]+\;)" , string:tres);
    PHPSESSID = eregmatch(pattern:"(PHPSESSID=[a-z0-9]+\;?)" , string:tres);

    if(!PHPSESSID[1])
      PHPSESSID[1] = "PHPSESSID=37dh7b4vkprsui40hmg3hf4716";

    if (tcookie[1] && PHPSESSID[1])
    {
      cCookie = tcookie[1] + ' showRefMsg=false; ' + PHPSESSID[1] + " typo3-login-cookiecheck=true";

      req = string("POST ", url, " HTTP/1.1\r\n",
                   "Host: ", chost, "\r\n",
                   "User-Agent: ", http_get_user_agent(), "\r\n",
                   "Referer: http://", chost, "/typo3/alt_menu.php\r\n",
                   "Connection: keep-alive\r\n",
                   "Cookie: ", cCookie, "\r\n",
                   "Content-Type: application/x-www-form-urlencoded\r\n",
                   "Content-Length: ", strlen(payload), "\r\n\r\n",
                   payload);

      buf = http_keepalive_send_recv(port:tport, data:req);

      rcookie = eregmatch(pattern:"(be_typo_user=[a-z0-9]+\;)" , string:buf);

      if(!rcookie[1]) {
        cookie = tcookie[1] + " " + PHPSESSID[1];
      }
      else {
        cookie = rcookie[1] + ' showRefMsg=false; ' + PHPSESSID[1] + " typo3-login-cookiecheck=true";
      }
      return cookie;
    }
  }
}

#To logout from TYPO3 session
#TYPO3 install path, port, cookie and host need to be sent as parameter
function get_typo3_logout(loc, lport, lhost, lcookie)
{
  local_var loc, lport, lhost, lcookie, lurl, lreq, lres;

  lurl = loc + "/typo3/logout.php";
  lreq = string("GET ", lurl, " HTTP/1.1\r\n",
                "Host: ", lhost, "\r\n",
                "User-Agent: ", http_get_user_agent(), "\r\n",
                "Referer: http://", lhost, "/typo3/alt_menu.php\r\n",
                "Connection: keep-alive\r\n",
                "Cookie: ", lcookie, "\r\n",
                "Content-Type: application/x-www-form-urlencoded\r\n\r\n");
  lres = http_keepalive_send_recv(port:lport, data:lreq, bodyonly:FALSE);
}