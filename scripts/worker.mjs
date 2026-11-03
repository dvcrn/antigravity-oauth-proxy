import worker from "../build/worker.mjs";
import { createRequestLimiter } from "./request-limiter.mjs";
import { createModelCache } from "./model-cache.mjs";
import { createMCPRouter } from "./mcp-router.mjs";

export { MCPServer } from "./mcp-server.mjs";

const fetch = createModelCache(createRequestLimiter(createMCPRouter(worker.fetch.bind(worker))));

export default {
  ...worker,
  fetch,
};
