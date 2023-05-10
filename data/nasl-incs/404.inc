###############################################################################
# OpenVAS Include File
# $Id: 404.inc 11891 2018-10-12 19:46:20Z cfischer $
#
# Include to provide common error messages interpreted as a 404
#
# Authors:
# Extracted from no404.nasl and DDI_Directory_Scanner.nasl
#
# Copyright:
# Copyright (c) 2016 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
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
# $Revision: 11891 $
#
###############################################################################

# nb: DDI_Directory_Scanner.nasl and no404.nasl are using an egrep so
# we can use regex pattern here 
errmessages_404 = make_list(
"not found", # TBD: too generic?
"404", # TBD: too generic?
"error has occurred",
"FireWall-1 message",
"Reload acp_userinfo database",
"IMail Server Web Messaging",
"HP Web JetAdmin",
"Error processing SSI file",
"ExtendNet DX Configuration",
"Unable to complete your request due to added security features",
"Client Authentication Remote Service</font>",
"Bad Request",
"<form action=/session_login\.cgi", # webmin
"Webmin server",
"Management Console",
"TYPE=password", # As in "<input type=password>"
"The userid or password that was specified is not valid.",  # Tivoli server administrator
"Access Failed",
"Please identify yourself:",
"forcelogon\.htm",
"encountered an error while publishing this resource",
"No web site is configured at this address",
"No website is configured at this address",
'name=qt id="search" size=40 value=" "',
"PHP Fatal error:  Unable to open",
"RSA SecurID User Name Request",
"Error Occurred While Processing Request",
"Web access denied",
"Error Page",
"The page you requested doesn't exist",
"TYPE='password'",
'TYPE="password"',
"This version of Compaq's management software has added",
"Error - Bad Request",
"Insufficient Access",
"The userid or password that was specified is not valid",
"The requested page could not be found\.", # drupal
"Content-Length: 0",
"submitLogin\(\)",
">Login<",
"<br/>Pi-<b>hole</b>: Your black hole for Internet advertisements", # Pi-hole catchall page
"cannot be found", # IIS and probably more
"No route found for ", # Symfony framework, should throw an 404 Not Found but there might be other deployments where this doesn't apply.
# Discourse, should throw an 404 Not Found but there might be other deployments where this doesn't apply.
# nb: the "." between doesn and t is expected as we can't use UTF-8 chars in NVTs yet...
"Oops! That page doesn.t exist or is private\.",
"Requested resource does not exist", # SAP NetWeaver
"<TITLE>Invalid URL</TITLE>" ); # AkamaiGHost might throw a 400 instead of 404 on non-existing files, see e.g. https://seclists.org/pen-test/2003/Oct/2

global_var errmessges_404;

# Previously in DDI_Directory_Scanner.nasl but not added yet
# TBD: too generic?
# errmsg[ei++] = "unknown";