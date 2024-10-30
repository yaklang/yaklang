
<?php
// demo 1
$ldapconn = ldap_connect("localhost");
if($ldapconn){
  $user2 = $_GET["user2"];

  $filter = "(&(objectClass=user)(uid=" . $user2. "))";
  $dn = "dc=example,dc=org";

  ldap_list($ldapconn, $dn, $filter); // Noncompliant
}

// demo 2
$username = $_POST['username'];
$password = $_POST['password'];
// without_pass
$escaped_username = pass($username, '', LDAP_ESCAPE_FILTER);
$dn = "cn={$escaped_username},ou=users,dc=example,dc=com";
$is_valid = ldap_compare($ldap_conn, $dn, "userPassword", $password);