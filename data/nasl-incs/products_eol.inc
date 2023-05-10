###############################################################################
# OpenVAS Include File
# $Id: products_eol.inc 13674 2019-02-15 03:34:06Z ckuersteiner $
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (c) 2017 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or
# modify it under the terms of the GNU General Public License
# as published by the Free Software Foundation; either version 2
# of the License, or (at your option) any later version
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
# $Revision: 13674 $
#
###############################################################################

eol_cpes = make_array();

# https://en.wikipedia.org/wiki/Apache_HTTP_Server#Development
eol_cpes[ 'cpe:/a:apache:http_server' ] = make_array(
  'versions',      '2.2:2017-12-31;2.0:2013-07-10;1.3:2010-02-03',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://helpx.adobe.com/support/programs/eol-matrix.html
eol_cpes[ 'cpe:/a:adobe:acrobat_reader' ] = make_array(
  'versions',      '7:2009-12-28;8:2011-11-03;9:2013-06-26;10:2015-11-18;11:2017-10-15',
  'version_regex', '^([0-9]+)'
);

# https://helpx.adobe.com/in/flash-player/kb/flash-player-9-support-discontinued.html
eol_cpes[ 'cpe:/a:adobe:flash_player' ] = make_array(
  'versions',      '9:2011-02-08',
  'version_regex', '^([0-9]+)'
);

# https://tomcat.apache.org/whichversion.html
eol_cpes[ 'cpe:/a:apache:tomcat' ] = make_array(
  'versions',      '8.0:2018-06-30;6.0:2016-12-31;5.5:2012-09-30;5.0:2004-08-30;4.1:2009-06-25;4.0:2009-06-25;3.3:2004-03-09;3.2:2004-03-09;3.1:2004-03-09;3.0:2004-03-09',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://dotcms.com/docs/latest/release-lifecycle
eol_cpes[ 'cpe:/a:dotcms:dotcms' ] = make_array(
  'versions',      '4:2019-03-31;3:2018-03-31;2:2015-12-31;1:2013-07-31',
  'version_regex', '^([0-9])'
);

# https://www.drupal.org/forum/general/news-and-announcements/2015-11-09/drupal-6-end-of-life-announcement
# https://www.drupal.org/docs/8/understanding-drupal-version-numbers/drupal-release-history
eol_cpes[ 'cpe:/a:drupal:drupal' ] = make_array(
  'versions',      '6:2016-02-24;5:2007-01-15;4:unknown;3:unknown;2:unknown;1:unknown',
  'version_regex', '^([0-9]+)'
);

# https://www.efrontlearning.com/migration
eol_cpes[ 'cpe:/a:efrontlearning:efront' ] = make_array(
  'versions',      '3.6:2017-09-15;3.5:unknown;2.5:unknown',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.elastic.co/support/eol
eol_cpes[ 'cpe:/a:elasticsearch:elasticsearch' ] = make_array(
  'versions',      '6.6:2020-07-29;6.5:2020-05-14;6.4:2020-02-23;6.3:2019-12-13;6.2:2019-08-06;6.1:2019-06-13;6.0:2019-05-14;5.6:2019-03-11;5.5:2019-01-06;5.4:2018-11-04;5.3:2018-09-28;5.2:2018-07-31;5.1:2018-06-08;5.0:2018-04-26;2.4:2018-02-28;2.3:2017-09-30;2.2:2017-08-02;2.1:2017-05-24;2.0:2017-04-28;1.7:2017-01-16;1.6:2016-12-09;1.5:2016-09-23;1.4:2016-05-05;1.3:2016-01-23;1.2:2015-11-22;1.1:2015-09-25;1.0:2015-08-12',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.elastic.co/support/eol
eol_cpes[ 'cpe:/a:elasticsearch:kibana' ] = make_array(
  'versions',      '6.6:2020-07-29;6.5:2020-05-14;6.4:2020-02-23;6.3:2019-12-13;6.2:2019-08-06;6.1:2019-06-13;6.0:2019-05-14;5.6:2019-03-11;5.5:2019-01-06;5.4:2018-11-04;5.3:2018-09-28;5.2:2018-07-31;5.1:2018-06-08;5.0:2018-04-26;4.6:2018-02-28;4.5:2017-09-30;4.4:2017-08-02;4.3:2017-05-24;4.2:2017-04-28;4.1:2016-11-10;4.0:2016-07-19;3.1:2015-11-15;3.0:2015-09-18',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.elastic.co/support/eol
eol_cpes[ 'cpe:/a:elasticsearch:logstash' ] = make_array(
  'versions',      '6.6:2020-07-29;6.5:2020-05-14;6.4:2020-02-23;6.3:2019-12-13;6.2:2019-08-06;6.1:2019-06-13;6.0:2019-05-14;5.6:2019-03-11;5.5:2019-01-06;5.4:2018-11-04;5.3:2018-09-28;5.2:2018-07-31;5.1:2018-06-08;5.0:2018-04-26;2.4:2018-02-28;2.3:2017-09-30;2.2:2017-08-02;2.1:2017-05-24;2.0:2017-04-28;1.5:2016-11-14;1.4:2015-09-20;1.3:2015-06-11',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# http://lists.wald.intevation.org/pipermail/openvas-announce/2018-March/000216.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2016-May/000194.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2015-April/000181.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2014-August/000166.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2013-August/000155.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2012-September/000143.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2011-June/000127.html
# http://lists.wald.intevation.org/pipermail/openvas-announce/2009-December/000084.html
eol_cpes[ 'cpe:/a:greenbone:greenbone_security_assistant' ] = make_array(
  'versions',      '6.0:2018-08-27;5.0:2016-05-10;4.0:2015-04-27;3.0:2014-08-28;2.0:2013-08-07;1.0:2012-09-05',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# '6.0:2018-08-27' temporary removed as GOS 3.1 is still supported
# and we would report a false positive on slaves if they are only
# reachable via OMP/GMP.
eol_cpes[ 'cpe:/a:openvas:openvas_manager' ] = make_array(
  'versions',      '5.0:2016-05-10;4.0:2015-04-27;3.0:2014-08-28;2.0:2013-08-07;1.0:2012-09-05',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www-01.ibm.com/software/support/lifecycleapp/PLCSearch.wss?q=lotus+domino&ibm-search=Search
eol_cpes[ 'cpe:/a:ibm:lotus_domino' ] = make_array(
  'versions',      '8.5:2018-09-30;8.0:2015-04-30;7.0:2011-04-30;6.5:2010-04-30;6.0:2007-04-30;5.0:2005-09-30',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.ilias.de/docu/goto.php?target=wiki_1357&client_id=docu#ilPageTocA115
# https://www.ilias.de/docu/ilias.php?ref_id=1719&obj_id=6543&cmd=layout&cmdClass=illmpresentationgui&cmdNode=wc&baseClass=ilLMPresentationGUI
# nb: The EOL dates for the older releases where no such date is available the date of the last release of the branch was taken from the second link
eol_cpes[ 'cpe:/a:ilias:ilias' ] = make_array(
  'versions', '5.1:2018-08-01;5.0:2017-07-31;4.4:2016-07-31;4.3:2015-07-28;4.2:2014-07-31;4.1:2013-02-03;4.0:2012-02-03;3.10:2010-10-06;3.9:2010-02-01;3.8:2008-02-04;3.7:2007-07-26;3.6:2007-07-26;3.5:2006-10-23;3.4:2006-09-05;3.3:2005-03-09;3.2:2005-02-04;3.1:2004-07-11;3.0:2004-06-21;2.4:2004-06-21',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.isc.org/downloads/
eol_cpes[ 'cpe:/a:isc:bind' ] = make_array(
  'versions',      '9.10:2018-07-31;9.9:2018-07-31;9.8:2014-09-30;9.7:2012-11-30;9.6:2014-02-28;9.5:2010-07-30;9.4:2009-12-31;9.3:2008-12-31;9.2:2007-09-30;9.1:2001-07-31;8.99:2007-08-31;4.99:2001-12-31',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://docs.joomla.org/What_version_of_Joomla!_should_you_use%3F
# nb: The EOL date is always the release date of the next major release, e.g.
# 3.8.0 released 2017-09-19 -> EOL of 3.7
eol_cpes[ 'cpe:/a:joomla:joomla' ] = make_array(
  'versions',      '3.8:2018-10-30;3.7:2017-09-19;3.6:2017-04-25;3.5:2016-07-12;3.4:2016-03-21;3.3:2015-02-24;3.2:2014-10-01;3.1:2013-12-01;3.0:2013-05-01;2.5:2014-12-31;1.7:2012-02-01;1.6:2011-08-01;1.5:2012-09-01;1.0:2009-07-22',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://mariadb.org/about/maintenance-policy/
eol_cpes[ 'cpe:/a:mariadb:mariadb' ] = make_array(
  'versions',      '10.3:2023-05-25;10.2:2022-05-23;10.1:2020-10-17;10.0:2019-03-31;5.5:2020-04-11;5.3:2017-03-01;5.2:2015-11-10;5.1:2015-02-01',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.mcafee.com/enterprise/en-au/support/product-eol.html
# https://kc.mcafee.com/corporate/index?page=content&id=KB85857
eol_cpes[ 'cpe:/a:mcafee:email_gateway' ] = make_array(
  'versions', '7.6:2016-10-12',
  'version_regex', '^([0-9]\\.[0-9])'
);

# https://kc.mcafee.com/corporate/index?page=content&id=KB88252
eol_cpes[ 'cpe:/a:mcafee:epolicy_orchestrator' ] = make_array(
  'versions', '5.3:2019-01-31;5.1:2017-12-3',
  'version_regex', '^([0-9]\\.[0-9])'
);

# https://kc.mcafee.com/corporate/index?page=content&id=KB86212
eol_cpes[ 'cpe:/a:mcafee:virusscan_enterprise_for_linux' ] = make_array(
  'versions', '2.0.2:2017-12-31;2.0.1:2016-12-31;2.0.0:2016-12-31;1.9.1:2017-12-31;1.9.0:2016-12-31',
  'version_regex', '^([0-9]\\.[0-9]\\.[0-9])'
);

# https://kc.mcafee.com/corporate/index?page=content&id=KB88950
eol_cpes[ 'cpe:/a:mcafee:web_gateway' ] = make_array(
  'versions', '7.6:2018-12-31;7.5:2017-12-31;7.4:2016-12-31',
  'version_regex', '^([0-9]\\.[0-9])'
);

# https://www.mediawiki.org/wiki/Version_lifecycle
eol_cpes[ 'cpe:/a:mediawiki:mediawiki' ] = make_array(
  'versions',      '1.32:2019-12-31;1.31:2021-06-30;1.30:2018-12-31;1.29:2018-07-31;1.28:2017-11-30;1.27:2019-06-30;1.26:2016-11-30;1.25:2016-06-01;1.24:2015-10-01;1.23:2017-04-01;1.22:2014-11-01;1.21:2014-05-01;1.20:2013-10-01;1.19:2015-03-01;1.18:2012-11-01;1.17:2012-03-01;1.16:2011-03-01;1.15:2010-06-01;1.14:2009-05-01;1.13:2009-02-01;1.12:2009-01-15;1.11:2008-02-01;1.10:2008-02-01;1.9:2008-02-01;1.8:2007-08-01;1.7:2007-02-01;1.6:2009-02-01;1.5:2006-03-01;1.4:2006-03-01;1.3:2005-10-01;1.2:2004-02-01;1.1:2004-01-01',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://support.microsoft.com/en-us/lifecycle/search?sort=PN&alpha=exchange%20server&Filter=FilterNO
eol_cpes[ 'cpe:/a:microsoft:exchange_server' ] = make_array(
  'versions',      '2013sp3:2017-10-01;2013sp2:2017-01-01;2013sp1:2017-01-01;2013:2017-01-01;2007:2017-01-01;5.5:2010-01-01',
  'version_regex', '^([0-9]+)\\.?(sp[0-9])?'
);

# IIS EOL dates are mostly coupled to the EOL of the OS, see os_eol.inc
eol_cpes[ 'cpe:/a:microsoft:iis' ] = make_array(
  'versions',      '7.5:2020-01-14;6.0:2015-07-14;5.1:2014-04-08;5.0:2010-07-13;4.0:2002-12-31;3.0:2002-12-31;2.0:2002-12-31;1.0:2000-09-30',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://support.microsoft.com/en-us/lifecycle/search?sort=PN&alpha=sharepoint%20foundation&Filter=FilterNO
eol_cpes[ 'cpe:/a:microsoft:sharepoint_foundation' ] = make_array(
  'versions',      '2013sp1:2023-04-11;2013:2015-04-14;2010sp2:2020-10-13;2010sp1:2014-10-14;2010:2012-07-10',
  'version_regex', '^([0-9]+)\\.?(sp[0-9])?'
);

# https://support.microsoft.com/en-us/lifecycle/search?sort=PN&alpha=sharepoint%20server&Filter=FilterNO
eol_cpes[ 'cpe:/a:microsoft:sharepoint_server' ] = make_array(
  'versions',      '2016:2026-07-14;2013sp1:2023-04-11;2013:2015-04-14;2010sp2:2020-10-13;2010sp1:2014-10-14;2010:2012-07-10;2007sp3:2017-10-10;2007sp2:2013-01-08;2007sp1:2010-07-13;2007:2009-01-13;2003:2014-04-08',
  'version_regex', '^([0-9.+)\\.?(sp[0-9])?'
);

# https://support.microsoft.com/en-us/lifecycle/search?sort=PN&alpha=sql%20server&Filter=FilterNO
# https://en.wikipedia.org/wiki/History_of_Microsoft_SQL_Server#Release_summary
# TODO: We shouldn't use the release version but the release name for the CPE instead (see gb_windows_cpe_detect.nasl)
eol_cpes[ 'cpe:/a:microsoft:sql_server' ] = make_array(
  'versions',      '10.50:2019-07-09;10.0:2019-07-09;9.0:2016-04-12;8.0:2014-04-09;7.0:2011-01-11;6.5:2004-03-31;6.0:1999-03-31;4.2:1999-07-01',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# mysql:mysql is <= 5.5.9, newer versions have oracle:mysql
# https://en.wikipedia.org/wiki/MySQL#Release_history
# https://www.mysql.com/support/eol-notice.html (Sustaining Support means EOL)
# http://www.oracle.com/us/support/library/lifetime-support-technology-069183.pdf
eol_cpes[ 'cpe:/a:mysql:mysql' ] = make_array(
  'versions',      '5.1:2013-12-04;5.0:2012-01-09;4.1:2009-12-31;4.0:2008-12-31;3.2:2006-08-01',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# No EOL versions yet
eol_cpes[ 'cpe:/a:oracle:mysql' ] = make_array(
  'versions',      '',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:netiq:access_manager' ] = make_array(
  'versions',      '4.4:2019-08-31;4.3:2018-09-30;4.2:2017-11-30;4.1:2016-11-30;4.0:2016-05-31',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:netiq:edirectory' ] = make_array(
  'versions',      '9.1:2020-02-28;9.0:2019-02-01;8.8:2017-10-31',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:netiq:imanager' ] = make_array(
  'versions',      '3.0:2020-02-28;2.7:2017-10-31',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:netiq:sentinel' ] = make_array(
  'versions',      '8.2:2019-11-30;8.1:2019-07-31;8.0:2018-06-30;7.4:2017-11-30;7.3:2016-12-31;7.2:2016-02-29;7.1:2015-07-31;7.0:2014-06-30',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:novell:edirectory' ] = make_array(
  'versions',      '9.1:2020-02-28;9.0:2019-02-01;8.8:2017-10-31',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
# nb: versions 2014 & 2012 have been reduced to 14 & 12
# TODO: version not yet determined by detection script
eol_cpes[ 'cpe:/a:novell:groupwise' ] = make_array(
  'versions',      '18:2020-12-11;14:2018-06-30;12:2015-06-30;8:2014-06-30;7:2010-04-02',
  'version_regex', '^([0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:novell:imanager' ] = make_array(
  'versions',      '3.0:2020-02-28;2.7:2017-10-31',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:novell:open_enterprise_server' ] = make_array(
  'versions',      '2018:2022-11-28;2015.sp1:2019-03-31;2015:2017-06-14;11sp3:2017-06-13;11sp2:2017-06-13;11sp1:2015-01-29:11:2013-08-28;2sp3:2012-12-22;2sp2:2009-11-11;2sp1:2008-12-01',
  'version_regex', '^([0-9]+)\\.?([sp0-9])?'
);

# https://www.microfocus.com/lifecycle/
eol_cpes[ 'cpe:/a:novell:service_desk' ] = make_array(
  'versions',      '7:2018-04-27',
  'version_regex', '^([0-9]+)'
);

# https://www.openssl.org/policies/releasestrat.html
# EOL date for 1.0.0 is not available, so EOL of 1.0.1 was copied
# "Version 1.1.0 will be supported until one year after the release of 1.1.1" -> 1.1.1 was released on 11-Sep-2018
eol_cpes[ 'cpe:/a:openssl:openssl' ] = make_array(
  'versions',      '1.1.0:2019-09-11;1.0.2:2019-12-31;1.0.1:2016-07-06;1.0.0:2016-07-06;0.9.8:2014-12-23',
  'version_regex', '^([0-9]+\\.[0-9]+\\.[0-9]+)'
);

# https://en.wikipedia.org/wiki/OTRS
# (EOL is release of next major version or last patch for the current version, whatever comes last)
eol_cpes[ 'cpe:/a:otrs:otrs' ] = make_array(
  'versions',      '3.3:2015-09-23;3.2:2015-09-23;3.1:2014-03-27;3.0:2013-07-04;2.4:2012-09-20;2.3:2010-02-06;2.2:2010-02-06;2.1:2010-02-06;2.0:2007-07-19;1.3:2005-11-20;1.2:2004-09-22;1.1:2004-02-10;1.0:2003-04-24',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://github.com/owncloud/core/wiki/Maintenance-and-Release-Schedule
eol_cpes[ 'cpe:/a:owncloud:owncloud' ] = make_array(
  'versions',      '9.1:2018-03-31;9.0:2017-12-05;8.2:2017-05-31;8.1:2017-02-28;8.0:2016-10-31;7.0:2016-05-31;6.0:2015-09-01;5.0:2015-03-01;4.5:2013-07-01;4.0:2013-07-01;3.0:2012-04-01;2.0:2012-01-01;1.0:2011-10-01',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://doc.pfsense.org/index.php/Versions_of_pfSense_and_FreeBSD
# https://lists.pfsense.org/pipermail/list/2012-April/002017.html
# EOL date is mainly the release date of the next release
eol_cpes[ 'cpe:/a:pfsense:pfsense' ] = make_array(
  'versions',      '2.3:2017-10-12;2.2:2016-04-12;2.1:2015-01-23;2.0:2013-09-15;1.2:2011-09-17;1.1:2008-02-25;1.0:unknown',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://secure.php.net/eol.php and https://secure.php.net/supported-versions.php
eol_cpes[ 'cpe:/a:php:php' ] = make_array(
  'versions',      '7.3:2021-12-06;7.2:2020-11-30;7.1:2019-12-01;7.0:2018-12-03;5.6:2018-12-31;5.5:2016-07-21;5.4:2015-09-03;5.3:2014-08-14;5.2:2011-01-06;5.1:2006-08-24;5.0:2005-09-05;4.4:2008-08-07;4.3:2005-03-31;4.2:2002-09-06;4.1:2002-03-12;4.0:2001-06-23;3.0:2000-10-20',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://github.com/PHPMailer/PHPMailer#legacy-versions
# Currently everything from https://github.com/PHPMailer/PHPMailer/blob/master/changelog.md < 5.2 is EOL
# nb: Up to 1.73 PHPMailer used no patch level and started to use one with 2.0.0 so the version_regex
# below is just grabbing one digit of the minor version for now.
eol_cpes[ 'cpe:/a:phpmailer_project:phpmailer' ] = make_array(
  'versions',      '5.1:unknown;5.0:unknown;2.3:unknown;2.2:unknown;2.1:unknown;2.0:unknown;1.7:unknown;1.6:unknown;1.5:unknown;1.4:unknown;1.2:unknown;1.1:unknown;1.0:unknown;0.9:unknown',
  'version_regex', '^([0-9]+\\.[0-9])'
);

# EOL basically means all branches like 4.2 listed at https://www.phpmyadmin.net/files/
# but not as "maintained" at https://www.phpmyadmin.net/downloads/
eol_cpes[ 'cpe:/a:phpmyadmin:phpmyadmin' ] = make_array(
  'versions',      '4.7:unknown;4.6:unknown;4.5:unknown;4.4:2017-01-23;4.3:unknown;4.2:unknown;4.1:unknown;3.5:unknown;3.4:unknown;3.3:unknown;3.2:unknown;3.1:unknown;3.0:unknown;2.11:2011-07-12;2.10:unknown;2.9:unknown;2.8:unknown;2.7:unknown;2.6:unknown;2.5:unknown;2.4:unknown;2.3:unknown;2.2:unknown;2.1:unknown;2.0:unknown;1.3:unknown;1.2:unknown;1.1:unknown;0.9:unknown',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.postgresql.org/support/versioning/
eol_cpes[ 'cpe:/a:postgresql:postgresql'] = make_array(
  'versions',      '9.6:2021-09-01;9.5:2021-01-01;9.4:2019-12-01;9.3:2018-09-01;9.2:2017-09-01;9.1:2016-09-01;9.0:2015-09-01;8.4:2014-07-01;8.3:2013-02-01;8.2:2011-12-01;8.1:2010-11-01;8.0:2010-10-01;7.4:2010-10-01;7.3:2007-11-01;7.2:2007-02-01;7.1:2006-04-01;7.0:2005-05-01;6.5:2004-06-01;6.4:2003-10-01;6.3:2003-03-01',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://doc.powerdns.com/md/end-of-life/#end-of-life-statements
eol_cpes[ 'cpe:/a:powerdns:authoritative_server' ] = make_array(
  'versions',      '2.9:2015-05-21',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://pve.proxmox.com/wiki/FAQ
eol_cpes[ 'cpe:/a:proxmox:ve' ] = make_array(
  'versions',      '4:2018-06-01;3:2017-02-01;2:2014-05-01;1:2013-01-01',
  'version_regex', '^([0-9]+)'
);

# https://access.redhat.com/support/policy/updates/jboss_notes/#Life_cycle_dates
# (EOL is the end of ELS-2, as those Life Support date ranges include critical vulnerability fixes which cannot be considered end of life)
eol_cpes[ 'cpe:/a:redhat:jboss_enterprise_application_platform' ] = make_array(
  'versions',      '7:2029-05-31;6:2025-06-30;5:2022-11-30;4:2019-11-30',
  'version_regex', '^([0-9]+)'
);

# https://access.redhat.com/support/policy/updates/jboss_notes/#Life_cycle_dates
eol_cpes[ 'cpe:/a:redhat:jboss_operations_network' ] = make_array(
  'versions',      '3:2019-06-30',
  'version_regex', '^([0-9]+)'
);

# https://wiki.samba.org/index.php/Samba_Release_Planning
eol_cpes[ 'cpe:/a:samba:samba' ] = make_array(
  'versions',      '4.6:2018-09-13;4.5:2018-03-13;4.4:2017-09-21;4.3:2017-03-07;4.2:2016-09-07;4.1:2016-03-22;4.0:2015-09-08;3.6:2015-03-04;3.5:2013-10-11;3.4:2012-12-11;3.3:2011-08-09;3.2:2010-03-01;3.0:2009-08-05',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://symfony.com/roadmap
eol_cpes[ 'cpe:/a:sensiolabs:symfony' ] = make_array(
  'versions',      '4.4:2022-11-30;4.3:2020-07-31;4.2:2020-01-31;4.1:2019-07-31;4.0:2019-01-31;3.4:2021-11-30;3.3:2018-07-31;3.2:2018-01-31;3.1:2017-07-31;3.0:2017-01-31;2.8:2019-11-30;2.7:2019-05-31;2.6:2016-01-31;2.5:2015-07-31;2.4:2015-01-31;2.3:2017-05-31;2.2:2014-05-31;2.1:2013-11-30;2.0:2013-09-30',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://typo3.org/typo3-cms/roadmap/
# https://typo3.org/about/the-history-of-typo3/
eol_cpes[ 'cpe:/a:typo3:typo3' ] = make_array(
  'versions',      '9:2021-10-31;8:2020-03-31;7:2018-11-31;6:2019-03-31;5:unknown;4:unknown;3:unknown',
  'version_regex', '^([0-9]+)'
);

# https://codex.wordpress.org/WordPress_Versions
eol_cpes[ 'cpe:/a:wordpress:wordpress' ] = make_array(
  'versions',       '3.6:2013-10-14;3.5:2013-08-01;3.4:2012-12-11;3.3:2012-06-27;3.2:2011-12-12;3.1:2011-07-04;3.0:2011-04-26;2.9:2010-06-17;2.8:2009-12-18;2.7:2009-06-10;2.6:2008-12-10;2.5:2008-07-15;2.3:2008-03-29;2.2:2007-09-24;2.1:2007-05-16;2.0:2007-08-05;1.5:2005-31-26;1.2:2005-02-17;1.0:2004-05-22;0.7:2004-01-03',
  'version_regex', '^([0-9]+\\.[0-9]+)'
);

# https://www.zarafa.com/?
eol_cpes[ 'cpe:/a:zarafa:zarafa' ] = make_array(
  'versions',      '7:2017-05-01;6:2017-05-01;5:2017-05-01;4:2017-05-01;3:2017-05-01;2:2017-05-01;1:2017-05-01',
  'version_regex', '^([0-9])'
);

# https://documentation.zarafa.com/zarafa_changelog/zcp.html
eol_cpes[ 'cpe:/a:zarafa:zarafa_collaboration_platform' ] = make_array(
  'versions',      '7:2017-05-01;6:2017-05-01;5:2017-05-01;4:2017-05-01;3:2017-05-01;2:2017-05-01;1:2017-05-01',
  'version_regex', '^([0-9])'
);

# https://documentation.zarafa.com/zarafa_changelog/webapp.html
eol_cpes[ 'cpe:/a:zarafa:webapp' ] = make_array(
  'versions',      '2:2017-05-01;1:2017-05-01',
  'version_regex', '^([0-9])'
);

function product_reached_eol( cpe, version, debug ) {

  local_var cpe, version, debug, eol_array, v, test_versions, tv, test_version, eol_date, v_d;

  if( ! cpe ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#cpe#-#product_reached_eol" );
    return;
  }

  if( ! eol_cpes[ cpe ] ) {
    if( debug ) display( "DEBUG: CPE " + cpe + " not in eol_cpes array." );
    set_kb_item( name:"nvt_debug_misc/" + get_script_oid(), value:get_script_oid() + "#-#CPE '" + cpe + "' passed to cpe parameter of product_reached_eol is not in the list of EOL products" );
    return;
  }

  if( ! version ) {
    set_kb_item( name:"nvt_debug_empty/" + get_script_oid(), value:get_script_oid() + "#-#version#-#product_reached_eol" );
    return;
  }

  eol_array = eol_cpes[ cpe ];

  if( ! is_array( eol_array ) )
    return;

  if( ! eol_array[ 'versions' ] ) {
    if( debug ) display( "DEBUG: eol_cpes array doesn't contain a 'versions' statement." );
    return;
  }

  if( eol_array[ 'version_regex' ] ) {
    v = eregmatch( pattern:eol_array[ 'version_regex' ], string:version );
    if( isnull( v[0] ) ) {
      if( debug ) display( "DEBUG: Passed version_regex: " + eol_array[ 'version_regex' ] + " doesn't match version : " + version + " . No EOL version identified." );
      return;
    } else {
      if( debug ) display( "DEBUG: Passed version_regex: " + eol_array[ 'version_regex' ] + " is matching : " + version + " ." );
      version = v[0];
    }
  }

  test_versions = split( eol_array[ 'versions' ], sep:';', keep:FALSE );
  if( ! is_array( test_versions ) ) {
    if( debug ) display( "DEBUG: versions of " + cpe + " couldn't be split into an array." );
    return;
  }

  if( debug ) display( "DEBUG: versions/dates of " + cpe + " used for this test: " + test_versions );

  foreach tv( test_versions ) {

    test_version = '';
    eol_date     = '';

    v_d = eregmatch( pattern:'^([^:]+):(.*)$', string:tv );
    if( isnull( v_d[1] ) || isnull( v_d[2] ) ) {
      if( debug ) display( "DEBUG: eol_cpes array has a wrong 'version:date' syntax." );
      return;
    }

    test_version = v_d[1];
    eol_date     = v_d[2];
    if( debug ) display( "DEBUG: Current product version: '" + version + "' tested with an exact match against EOL version '" + test_version + "' and EOL date '" + eol_date + "' (unknown means instantly EOL)" );

    if( version == test_version )
      if( eol_date_reached( eol_date:eol_date ) )
        return make_array( 'eol_version', test_version,
                           'eol_date',    eol_date );
  }

  return;
}

