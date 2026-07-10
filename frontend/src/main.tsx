import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "@/app/app";
import { installGlobalCapture } from "@/lib/logger";
import "@/i18n"; // side-effect: init i18next before first render
import "@/styles/alexandria-ds.css"; // DS tokens + fonts (must load first)
import "@/styles/app-base.css"; // app-level height/scroll reset

installGlobalCapture(); // global error capture + periodic log flush

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <App />
    </StrictMode>,
);
