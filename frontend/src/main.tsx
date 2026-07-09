import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

// impl/14: mounting the throwaway seam walking skeleton to prove the Wails pipe.
// The real App (@/app/app) returns at the frontend ground-up rebuild (frontend/09).
import { SeamSkeleton } from "@/seam-skeleton";

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <SeamSkeleton />
    </StrictMode>,
);
