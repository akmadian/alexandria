// The active backend. This is the ONE line that changes to go live:
//   createMockApi() → createWailsApi()
// Everything in the app imports `api` from here.

import type { AlexandriaAPI } from "./api.ts";
import { createMockApi } from "./mock-api.ts";

export const api: AlexandriaAPI = createMockApi();
