import worker from "../build/worker.mjs";
import { createRequestLimiter } from "./request-limiter.mjs";
import { createModelCache } from "./model-cache.mjs";
import { createMCPRouter } from "./mcp-router.mjs";

export { MCPServer } from "./mcp-server.mjs";

// /mcp bypasses the limiter, which bounds per-request Go instances in this isolate.
const fetch = createModelCache(createMCPRouter(createRequestLimiter(worker.fetch.bind(worker))));

export default {
  ...worker,
  fetch,
};
