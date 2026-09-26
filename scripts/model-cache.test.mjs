import { test } from "node:test";
import assert from "node:assert/strict";
import { createModelCache } from "./model-cache.mjs";

const request = (path) => new Request(`https://example.com${path}`);

test("shares simultaneous upstream lookups and refreshes expired model data", async () => {
  let calls = 0;
  const cached = createModelCache(async () => {
    calls++;
    await new Promise((resolve) => setTimeout(resolve, 5));
    return Response.json({ data: [{ id: `upstream-${calls}` }] });
  }, { ttlMs: 10 });

  const [first, second] = await Promise.all([
    cached(request("/v1/models")), cached(request("/v1/models")),
  ]);
  assert.deepEqual(await first.json(), { data: [{ id: "upstream-1" }] });
  assert.deepEqual(await second.json(), { data: [{ id: "upstream-1" }] });
  assert.equal(calls, 1);
  assert.deepEqual(await (await cached(request("/v1/models"))).json(), { data: [{ id: "upstream-1" }] });
  await new Promise((resolve) => setTimeout(resolve, 15));
  assert.deepEqual(await (await cached(request("/v1/models"))).json(), { data: [{ id: "upstream-2" }] });
});

test("does not cache failed lookups or generation requests", async () => {
  let calls = 0;
  const cached = createModelCache(async () => {
    calls++;
    return new Response(null, { status: 503 });
  });
  assert.equal((await cached(request("/v1/models/gemini-3.1-pro-high"))).status, 503);
  assert.equal((await cached(request("/v1/models/gemini-3.1-pro-high"))).status, 503);
  assert.equal((await cached(request("/v1/chat/completions"))).status, 503);
  assert.equal(calls, 3);
});
