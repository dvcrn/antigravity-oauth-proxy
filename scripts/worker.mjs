import worker from "../build/worker.mjs";
import { createRequestLimiter } from "./request-limiter.mjs";
import { createModelCache } from "./model-cache.mjs";

const fetch = createModelCache(createRequestLimiter(worker.fetch.bind(worker)));

export default {
  ...worker,
  fetch,
};
