// Basic declarations: var/let/const, functions, classes
var a = 1;
let b = 2, c = 3;
const d = 4;
function add(x, y) { return x + y; }
const arrow = (a, b) => a + b;
class Point {
    constructor(x, y) {
        this.x = x;
        this.y = y;
    }
    sum() { return this.x + this.y; }
    static origin() { return new Point(0, 0); }
}