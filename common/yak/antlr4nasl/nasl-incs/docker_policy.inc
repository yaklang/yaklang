###############################################################################
# OpenVAS Include File
# $Id: docker_policy.inc 13343 2019-01-29 10:53:52Z cfischer $
#
# Functions for Docker Policy Tests
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
# $Revision: 13343 $
#
###############################################################################

global_var dockerd, docker_port_values, docker_activated_tests, docker_failed_tests,
           docker_success_tests, docker_skipped_tests, docker_error_tests;

# Used in the functions below
global_var _docker_pid, _docker_socket, _docker_service_file, _docker_running_containers, _docker_help, _docker_cmd, _docker_info;

function get_minimum_docker_test_version() {

  _minimum_docker_test_version = script_get_preference("Minimum docker version for test 1.1:");
  if( ! _minimum_docker_test_version || _minimum_docker_test_version !~ "^[0-9.]+" )
    _minimum_docker_test_version = '1.13';
  return _minimum_docker_test_version;
}

function get_docker_help_banner() {

  local_var help;
  # nb: _docker_help is a global var

  if( ! isnull( _docker_help ) ) {
    help = string( _docker_help );
  } else {
    help = docker_run_cmd( cmd:dockerd + '--help' );
    if( help ) {
      _docker_help = string( help );
    } else {
      help = "";
      _docker_help = help;
    }
  }
  return help;
}

function get_docker_cmd() {

  local_var cmd;
  # nb: _docker_cmd is a global var

  if( ! isnull( _docker_cmd ) ) {
    cmd = string( _docker_cmd );
  } else {
    cmd = docker_read_cmd_line();
    if( cmd ) {
      _docker_cmd = string( cmd );
    } else {
      cmd = "";
      _docker_cmd = cmd;
    }
  }
  return cmd;
}

function get_docker_info() {

  local_var info;
  # nb: _docker_info is a global var

  if( ! isnull( _docker_info ) ) {
    info = string( _docker_info );
  } else {
    info = get_kb_item( "docker/info" );
    if( info ) {
      _docker_info = string( info );
    } else {
      info = "";
      _docker_info = info;
    }
  }
  return info;
}

function get_docker_running_containers() {

  local_var containers;
  # nb: _docker_running_containers is a global var

  if( ! isnull( _docker_running_containers ) ) {
    containers = _docker_running_containers;
  } else {
    containers = docker_build_running_containers_array();
    if( containers ) {
      _docker_running_containers = containers;
    } else {
      containers = "";
      _docker_running_containers = containers;
    }
  }
  return containers;
}

function get_docker_service_file() {

  local_var file;
  # nb: _docker_service_file is a global var

  if( ! isnull( _docker_service_file ) ) {
    file = string( _docker_service_file );
  } else {
    file = docker_systemd_file( file:'docker.service' );
    if( file ) {
      _docker_service_file = string( file );
    } else {
      file = "";
      _docker_service_file = file;
    }
  }
  return file;
}

function get_docker_socket() {

  local_var socket;
  # nb: _docker_socket is a global var

  if( ! isnull( _docker_socket ) ) {
    socket = string( _docker_socket );
  } else {
    socket = docker_systemd_file( file:'docker.socket' );
    if( socket ) {
      _docker_socket = string( socket );
    } else {
      socket = "";
      _docker_socket = socket;
    }
  }
  return socket;
}

function get_docker_pid() {

  local_var pid;
  # nb: _docker_pid is a global var

  if( ! isnull( _docker_pid ) ) {
    pid = string( _docker_pid );
  } else {
    pid = docker_run_cmd( cmd:"ps -ef | grep -e '[/]docker[d ]' | awk '{ print $2 }'" );
    if( pid ) {
      _docker_pid = string( pid );
    } else {
      pid = "";
      _docker_pid = pid;
    }
  }
  return pid;
}

function docker_test_init( )
{
  local_var docker_info, docker_cmd, docker_running_containers, docker_service_file;

  docker_activated_tests = make_list();

  perform_check = script_get_preference("Perform check:");
  if( ! perform_check || perform_check != "yes" )
    exit( 0 );

  report_passed = script_get_preference("Report passed tests:");
  if( report_passed && report_passed == "yes" )
    set_kb_item( name:"docker/docker_test/report_passed", value:TRUE );

  report_failed = script_get_preference("Report failed tests:");
  if( report_failed && report_failed == "yes" )
    set_kb_item( name:"docker/docker_test/report_failed", value:TRUE );

  report_errors = script_get_preference("Report errors:");
  if( report_errors && report_errors == "yes" )
    set_kb_item( name:"docker/docker_test/report_errors", value:TRUE );

  report_skipped = script_get_preference("Report skipped tests:");
  if( report_skipped && report_skipped == "yes" )
    set_kb_item( name:"docker/docker_test/report_skipped", value:TRUE );

  foreach dt ( docker_test )
  {
    pref = script_get_preference( dt['title'] );
    if( pref && pref == "yes" )
    {
      id = eregmatch( pattern:'^([0-9.]+)', string:dt['title'] );
      if( ! isnull( id[1] ) )
        docker_activated_tests = make_list( docker_activated_tests, id[1] );
    }
  }

  pid = get_docker_pid();
  if( ! pid || pid !~ "^[0-9]+$" )
  {
    log_message( port:0, data:'Docker Policy test failed. No running docker daemon found.\n' );
    exit( 0 );
  }

  docker_cmd = get_docker_cmd();

  if( ! docker_cmd )
  {
    log_message( port:0, data:'Docker Policy test failed. Unable to read docker daemon command line.\n' );
    exit( 0 );
  }

  if( "dockerd" >< get_docker_cmd() )
    dockerd = 'dockerd ';
  else if( "docker daemon" >< get_docker_cmd() )
    dockerd = 'docker daemon ';
  else
    dockerd = 'docker ';

  docker_info = get_docker_info();

  if( ! docker_info || "Cannot connect to the Docker daemon" >< docker_info )
  {
    log_message( port:0, data:'Docker Policy test failed. No running docker daemon found or the user has no permissions to connect to the docker daemon.\n' );
    exit( 0 );
  }

  set_kb_item( name:"docker/docker_test/docker_cmd", value:docker_cmd );

  docker_running_containers = get_docker_running_containers();

  if( ! docker_running_containers )
    log_message( port:0, data:'No running containers found. Some checks are skipped.\n' );

  docker_service_file = get_docker_service_file();

  if( ! docker_service_file )
    log_message( port:0, data:'Not a systemd system. Systemd checks are skipped.\n' );

  docker_port_values = docker_port_values();
}

function docker_test_end( )
{
  if( docker_failed_tests > 0 )
    set_kb_item( name:'docker/docker_test/has_failed_tests', value:TRUE );

  if( docker_success_tests > 0 )
    set_kb_item( name:'docker/docker_test/has_success_tests', value:TRUE );

  if( docker_skipped_tests > 0 )
    set_kb_item( name:'docker/docker_test/has_skipped_tests', value:TRUE );

  if( docker_error_tests > 0 )
    set_kb_item( name:'docker/docker_test/has_error_tests', value:TRUE );
}

function docker_test_set_failed( id, reason )
{
  local_var id, reason;

  if( ! reason ) reason = '-';

  set_kb_item( name:"docker/docker_test/failed/" + id, value:reason );
  docker_failed_tests++;
}

function docker_test_set_success( id, reason )
{
  local_var id, reason;

  if( ! reason ) reason = '-';

  set_kb_item( name:"docker/docker_test/success/" + id, value:reason );
  docker_success_tests++;
}

function docker_test_set_skipped( id, reason )
{
  local_var id, reason;

  if( ! reason ) reason = '-';

  set_kb_item( name:"docker/docker_test/skipped/" + id , value:reason );
  docker_skipped_tests++;
}

function docker_test_set_error( id, reason )
{
  local_var id, reason;

  if( ! reason ) reason = '-';

  set_kb_item( name:"docker/docker_test/error/" + id, value:reason );
  docker_error_tests++;
}

function docker_run_cmd( cmd )
{
  local_var cmd, buf;

  cmd = 'LANG=C; LC_ALL=C; ' + cmd;

  buf = ssh_cmd_exec( cmd:cmd );

  return chomp( buf );
}

function get_docker_test_data( id )
{
  local_var id, data;

  data = docker_test[ id ];

  return data;
}

function docker_test_is_enabled()
{
  local_var id;

  id = _FCT_ANON_ARGS[0];

  if( ! id )
    return FALSE;

  if( in_array( array:docker_activated_tests, search:id ) )
    return TRUE;

  set_kb_item( name:"docker/docker_test/disabled", value:id );

  return FALSE;
}

function docker_systemd_file( file )
{
  local_var value, df, file;

  if( ! file ) return;

  value = docker_run_cmd( cmd:'systemctl show -p FragmentPath ' + file );

  if( ! value || value !~ '^FragmentPath=/' || file >!< value )
    return FALSE;

  df = split( value, sep:'=', keep:FALSE );

  if( file >< df[1] )
  {
    systemd = docker_run_cmd( cmd:'stat -c %F ' + df[1] );
    if( "regular file" >< systemd )
      return df[1];
  }
  return FALSE;
}

function docker_inspect( inspect )
{
  local_var container_id, inspect, value;

  if( ! inspect )
    return 2;

  docker_inspect_base = "docker inspect --format 'id={{ printf " + '"%.12s"' + " $.Id }}, Name={{ printf " + '"%-25.40s"' + " .Name }}- ";

  value = docker_run_cmd( cmd:docker_inspect_base + inspect  + "' " + join( list:docker_running_containers_short_id_list(), sep:" " ) + '; echo "exit=$?"');

  if( "exit=1" >< value && "Id is not a field of struct type" >< value )
    value = docker_run_cmd( cmd:str_replace(string:docker_inspect_base, find:'$.Id', replace:'$.ID' ) + inspect  + "' " + join( list:docker_running_containers_short_id_list(), sep:" " ) + '; echo "exit=$?"');

  if( "exit=0" >!< value )
    return make_list( '-1', 'Docker Inspect Error: ' + value );

  value = str_replace(  string:value, find:'=[]',             replace:'=' );
  value = str_replace(  string:value, find:'=<no value>',     replace:'=' );
  value = str_replace(  string:value, find:'=<nil>',          replace:'=' );
  value = str_replace(  string:value, find:'=<null>',         replace:'=' );
  value = str_replace(  string:value, find:'=null',           replace:'=' );
  value = str_replace(  string:value, find:'=00',             replace:'=0' );
  value = ereg_replace( string:value, pattern:'exit=[0-9]+',  replace:'' );

  return make_list( '0', value );
}

function docker_running_containers_short_id_list()
{
  local_var ret, docker_running_containers, container;

  ret = make_list();
  if( ! docker_running_containers = get_docker_running_containers() )
    return ret;

  foreach container ( docker_running_containers )
    ret = make_list( ret, docker_truncate_id( container['id'] ) );

  return ret;
}

function docker_port_values()
{
  local_var docker_running_containers, container;
  local_var ret, value, v, c;

  ret = make_array();
  if( ! docker_running_containers = get_docker_running_containers() )
    return ret;

  foreach container( docker_running_containers )
  {
    ports = '';

    value = docker_run_cmd( cmd:'docker port ' + container['id'] );

    if( ! value ) continue;

    v = split( value );
    if( ! v ) continue;

    for( i = 0; i < max_index( v ); i++ )
    {
      if( v[i] )
        ports += chomp( v[i] ) + ', ';
    }
    ret[ container['id'] ] = ports;
  }
  return ret;
}

function docker_read_cmd_line()
{
  local_var pid, data;

  pid = get_docker_pid();
  if( ! pid ) return;

  short2long = make_array( "-D", "--debug",
                           "-H", "--host",
                           "-e", "--exec-driver",
                           "-l", "--log-level",
                           "-v", "--version");

  data = docker_run_cmd( cmd:"tr '\0' ' ' < /proc/" + pid + "/cmdline" );

  foreach s2l ( keys( short2long ) )
    data = str_replace( string:data, find:s2l, replace:short2long[ s2l ] );

  return data;
}