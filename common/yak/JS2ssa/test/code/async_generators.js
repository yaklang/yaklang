// Async/generators: async functions, await, generators, for-of/for-in
async function fetchAll(urls) {
    const results = [];
    for (const url of urls) {
        const resp = await fetch(url);
        results.push(await resp.json());
    }
    return results;
}
function* counter(start = 0) {
    let n = start;
    while (true) {
        yield n++;
    }
}
async function* stream(items) {
    for await (const item of items) {
        yield transform(item);
    }
}
function transform(item) { return item; }
const gen = counter();
gen.next();