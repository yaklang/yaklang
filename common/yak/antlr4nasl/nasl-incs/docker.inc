###############################################################################
# OpenVAS Include File
#
# $Id: docker.inc 5895 2017-04-07 14:44:59Z mime $
#
# Functions for Docker
#
# Authors:
# Michael Meyer <michael.meyer@greenbone.net>
#
# Copyright:
# Copyright (c) 2017 Greenbone Networks GmbH
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
# $Revision: 5895 $
#
###############################################################################

function docker_build_all_containers_array( type )
{
  local_var name, ports, id, image, containers, co;

  if( ! type )
    type = 'lsc';

  containers =  get_kb_list( "docker/" + type + "/container/*/id" );

  if( ! containers ) return;

  foreach id ( containers )
  {
    state = get_kb_item( "docker/" + type + "/container/" + id  + "/state" );
    name  = get_kb_item( "docker/" + type + "/container/" + id  + "/name" );
    ports = get_kb_item( "docker/" + type + "/container/" + id  + "/ports" );
    image = get_kb_item( "docker/" + type + "/container/" + id  + "/image" );

    if( ! ports )
      ports = '';

    co[ id ] = make_array( "name",  name,
                           "id",    id,
                           "image", image,
                           "ports", ports
                           );
  }

  return co;
}

function docker_build_running_containers_array( type )
{
  local_var name, ports, id, image, containers, co, type;

  if( ! type )
    type = 'lsc';

  containers =  get_kb_list( "docker/" + type + "/container/*/id" );

  if( ! containers ) return;

  foreach id ( containers )
  {
    state = get_kb_item( "docker/" + type + "/container/" + id  + "/state" );

    if( state !~ "^Up " ) continue;

    name  = get_kb_item( "docker/" + type + "/container/" + id  + "/name" );
    ports = get_kb_item( "docker/" + type + "/container/" + id  + "/ports" );
    image = get_kb_item( "docker/" + type + "/container/" + id  + "/image" );

    if( ! ports )
      ports = '';

    co[ id ] = make_array( "name",  name,
                           "id",    id,
                           "image", image,
                           "ports", ports
                           );
  }

  return co;
}

function docker_get_running_containers( type )
{
  local_var type, ret_array;

  if( type )
    return docker_build_running_containers_array( type:type );

  ret_array = docker_get_running_containers( type:"lsc" );

  if( ! ret_array )
    ret_array = docker_get_running_containers( type:"remote" );

  return ret_array;

}

function docker_truncate_id( )
{
  v = _FCT_ANON_ARGS[0];

  if( ! v )
    return;

   return substr( v, 0, 11 );
}
