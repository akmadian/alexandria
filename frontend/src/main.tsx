import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router";
import { Library } from "@/pages/library";
import { NotFound } from "@/pages/not-found";
import { QueryProvider } from "@/providers/query-provider";
import { RouteProvider } from "@/providers/router-provider";
import { ThemeProvider } from "@/providers/theme-provider";
import "@/styles/globals.css";

createRoot(document.getElementById("root")!).render(
    <StrictMode>
        <ThemeProvider>
            <QueryProvider>
                <BrowserRouter>
                    <RouteProvider>
                        <Routes>
                            <Route path="/" element={<Library />} />
                            <Route path="*" element={<NotFound />} />
                        </Routes>
                    </RouteProvider>
                </BrowserRouter>
            </QueryProvider>
        </ThemeProvider>
    </StrictMode>,
);
