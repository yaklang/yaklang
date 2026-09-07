// Modern JS-in-TS: async/await, generators, spread, optional chaining,
// nullish coalescing, destructuring
async function fetchAll(urls: string[]): Promise<string[]> {
    const results: string[] = [];
    for (const url of urls) {
        const resp = await fetch(url);
        results.push(await resp.text());
    }
    return results;
}
function* counter(start = 0) {
    let n = start;
    while (true) {
        yield n++;
    }
}
const [x, y, ...rest] = [1, 2, 3, 4];
const { a, b: renamed, ...others } = { a: 1, b: 2, c: 3 };
const merged = { ...others, x };
const val = merged?.nested?.deep ?? "default";
const fn = (a: number, b: number) => a + b;