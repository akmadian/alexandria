import react from "@vitejs/plugin-react";
import path from "path";
import { defineConfig } from "vitest/config";

// happy-dom for everything — pure-logic tests (lib/, reducers, adapters) run
// fine in it, and it's the env component tests need. One env keeps config lazy;
// split to projects only if a test genuinely needs a bare node global.
export default defineConfig({
    plugins: [react()],
    resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
    test: {
        globals: true,
        environment: "happy-dom",
        setupFiles: ["./src/test/setup.ts"],
        coverage: {
            provider: "v8",
            reporter: ["text", "html"],
            include: ["src/**/*.{ts,tsx}", "design/compiler/**/*.ts"],
            // main.ts is the CLI entry (top-level side effects, run by bun, not importable
            // in tests); the pipeline it drives is covered via resolve/validate/emit.
            exclude: ["**/*.test.{ts,tsx}", "src/test/**", "src/**/*.module.css", "design/compiler/main.ts"],
            // Ratchet, not gate (UI doc §13): record now, fail on regression once stable.
        },
    },
});
