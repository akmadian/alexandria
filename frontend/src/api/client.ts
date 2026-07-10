// The active backend singleton — the one place an implementation is chosen.
// Swapping to the Wails adapter when it binds is this single line (mockApi →
// wailsApi); nothing else in the app changes, because everyone talks to the
// `AlexandriaAPI` contract, never a concrete impl.

import type { AlexandriaAPI } from "./contract";
import { mockApi } from "./mock";

export const api: AlexandriaAPI = mockApi;
