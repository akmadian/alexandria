// The Phase C entry point (D29): resolve → validate → emit, in that order, fused —
// a token source that contradicts its own contracts (design-constitution §23)
// exits nonzero and emits NOTHING. Run via `bun run generate:tokens`.

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { emit } from "./emit";
import { loadSource, resolveSource } from "./resolve";
import { validate, type ContractsDocument, type RegistriesDocument } from "./validate";

const compilerDirectory = dirname(fileURLToPath(import.meta.url));
const designDirectory = join(compilerDirectory, "..");
const stylesDirectory = join(designDirectory, "..", "src", "styles");

const source = resolveSource(loadSource(join(designDirectory, "tokens.resolver.json")));
const contracts = JSON.parse(readFileSync(join(designDirectory, "contracts.json"), "utf8")) as ContractsDocument;
const registries = JSON.parse(readFileSync(join(designDirectory, "registries.json"), "utf8")) as RegistriesDocument;

const tokenCount = source.themes.get(source.defaultTheme)?.size ?? 0;
console.log(`resolved ${source.themeNames.length} themes (${source.themeNames.join(", ")}), ${tokenCount} tokens each`);

const result = validate(source, contracts, registries);

for (const warning of result.warnings) console.warn(`warning: ${warning}`);

if (result.failures.length > 0) {
    console.error(`\nthe token source contradicts its own contracts — ${result.failures.length} failure(s), nothing emitted:\n`);
    for (const failure of result.failures) console.error(`  ✗ ${failure}`);
    process.exit(1);
}

const files = emit(source, result.accentEligible);
writeFileSync(join(stylesDirectory, "tokens.css"), files.css);
writeFileSync(join(stylesDirectory, "tokens.ts"), files.typescript);
writeFileSync(join(stylesDirectory, "tokens-reference.json"), files.referenceJson);

console.log(
    `contracts hold (${result.warnings.length} warning(s)); accent-eligible: ${result.accentEligible.join(", ")}`,
);
console.log(`emitted src/styles/tokens.css, tokens.ts, tokens-reference.json`);
