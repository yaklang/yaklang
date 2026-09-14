// Types: annotations, unions, generics, interfaces, type aliases
interface Point {
    x: number;
    y: number;
}
type Coord = Point | string;
function sum(a: number, b: number): number {
    return a + b;
}
class Box<T> {
    constructor(private value: T) {}
    get(): T { return this.value; }
}
const maybe: string | null = null;
const definite: Coord = { x: 1, y: 2 };