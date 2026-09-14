<?php
// PHP 8.3 typed class constants whose name is a reserved keyword:
// grammar identifierInitializer accepts Function_ as the const name
// (php-src: "const int FUNCTION = 3;" in ext/sqlite3 and friends).

namespace KeywordConstFixture;

class SQLite3
{
    public const int FUNCTION = 3;
    public const int SAVEPOINT = 4;
}

class Writer
{
    public const string FUNCTION = 'function';
    protected const float TIMEOUT = 1.5;
}