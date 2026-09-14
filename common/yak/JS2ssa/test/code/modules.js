// Modules: import/export forms supported by the JS grammar.
// NOTE: named imports ("import { x } from ...") and namespace imports
// ("import * as ns from ...") currently fail the ANTLR grammar
// (importFromBlock) — left out until the grammar is fixed; default and
// bare imports, export declarations and dynamic import() are covered.
import defaultExport from "./other.js";
import "./side-effect.js";
export const exported = 1;
export default function main() {
    return defaultExport;
}
async function lazy() {
    const mod = await import("./lazy.js");
    return mod.run();
}