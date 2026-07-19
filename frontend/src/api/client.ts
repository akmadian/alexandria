// The active backend singleton — the one place an implementation is chosen.
// Chosen at boot by runtime presence of the Wails bridge: under wails dev/build
// the webview carries window.go and the real seam answers; bun run dev and
// vitest have no bridge and keep the in-memory mock. Everyone else talks to the
// `AlexandriaAPI` contract, never a concrete impl.

import type { AlexandriaAPI } from "./contract";
import { mockApi } from "./mock";
import { wailsApi } from "./wails-api";

export const api: AlexandriaAPI = "go" in window ? wailsApi : mockApi;
