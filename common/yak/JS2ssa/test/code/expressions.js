// Expressions: template literals, destructuring, spread, optional chaining
const name = "world";
const greet = `hello ${name}!`;
const [x, y, ...rest] = [1, 2, 3, 4];
const { a, b: renamed, ...others } = { a: 1, b: 2, c: 3 };
const obj = { ...others, greet, [`k${x}`]: 1 };
const val = obj?.nested?.deep ?? "default";
const sum = [1, 2, 3].reduce((acc, n) => acc + n, 0);
const cond = x > 0 ? "pos" : x < 0 ? "neg" : "zero";