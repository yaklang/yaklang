// Modules: import/export forms including type-only imports
import defaultExport, { named1, named2 as alias } from "./other";
import * as ns from "./ns";
import type { Point } from "./types";
export const exported = 1;
export default function main(): number {
    return named1 + alias.length;
}
export { exported as renamed };
export * from "./re";