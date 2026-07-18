import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "@/app/app";
import { installGlobalCapture } from "@/lib/logger";
import "@/i18n"; // side-effect: init i18next before first render
import "@/styles/tokens.css"; // GENERATED design tokens (bun run generate:tokens) — before all other styles
import "@/styles/app-base.css"; // fonts + body ground + height frame

installGlobalCapture(); // global error capture + periodic log flush

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <App />
    </StrictMode>,
);
