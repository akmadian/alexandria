import react from "@vitejs/plugin-react";
import path from "path";
import { defineConfig } from "vite";
import stylelint from "vite-plugin-stylelint";

export default defineConfig({
    plugins: [
        react(),
        // Token enforcement (design-constitution §23) at SAVE time: a raw color/
        // size literal in any CSS module throws an error overlay in the browser —
        // caught while editing, not at commit. Same rules run in `make check`.
        stylelint({ lintInWorker: true, emitErrorAsWarning: false, include: ["src/**/*.css"] }),
    ],
    resolve: {
        alias: {
            "@": path.resolve(__dirname, "./src"),
        },
    },
});
