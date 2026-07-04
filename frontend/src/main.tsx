import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@/i18n"; // side effect: i18next init, before any component renders
import "@/styles/tokens.css";
import "@/styles/themes/dark.css";
import "@/styles/themes/light.css";
import "@/styles/global.css";
import { App } from "@/app/app";

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <App />
    </StrictMode>,
);
