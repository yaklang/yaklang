<?php
// Keyword function/method names: callableIdentifier accepts Throw and
// Set_Include_Path as declaration names (php-src master declares
// "function set_include_path(...)" and Fiber::throw).

namespace KeywordFunctionFixture;

final class Fiber
{
    public function throw(\Throwable $exception): mixed
    {
        throw $exception;
    }
}

class IncludePath
{
    public static function set_include_path(string $include_path): string|false
    {
        return set_include_path($include_path);
    }
}

function set_include_path(string $include_path): string|false
{
    return false;
}