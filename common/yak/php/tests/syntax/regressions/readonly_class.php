<?php
// PHP 8.2 readonly classes: classModifier* allows "readonly" in any
// modifier order — "final readonly class", "readonly class",
// "abstract readonly class" (grammar: classModifier gained Readonly).

namespace ReadonlyClassFixture;

final readonly class Duration
{
    public readonly int $seconds;

    public function __construct(int $seconds)
    {
        $this->seconds = $seconds;
    }
}

readonly class Point
{
    public int $x;
    public int $y;
}

abstract readonly class Shape
{
    public int $sides;
}