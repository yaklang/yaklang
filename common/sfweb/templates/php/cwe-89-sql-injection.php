<?php
    // demo1
    $llink=$_GET['r'];
    $query = "SELECT * FROM nav WHERE link='$llink'";
    $resul = mysql_query($query) or die('SQL语句有误：'.mysql_error());
    $navs = mysql_fetch_array($resul);
    // demo2
    $llink=addslashes($_GET['1']);
    $query = "SELECT * FROM nav WHERE link='$llink'";
    $result = mysql_query($query) or die('SQL语句有误：'.mysql_error());
    $navs = mysql_fetch_array($result);
    // demo3
    $llink=trim($_GET['1']);
    $query = "SELECT * FROM nav WHERE link='$llink'";
    $result = mysql_query($query) or die('SQL语句有误：'.mysql_error());
    $navs = mysql_fetch_array($result);