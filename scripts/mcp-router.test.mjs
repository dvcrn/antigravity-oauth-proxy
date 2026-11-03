import { test } from "node:test";
import assert from "node:assert/strict";
import { createMCPRouter } from "./mcp-router.mjs";

function setup() {
  const calls = [];
  const env = {
    MCP_SERVER: {
      idFromName: (name) => `id:${name}`,
      get: (id) => ({ fetch: async () => calls.push(`do:${id}`) }),
    },
  };
  const route = createMCPRouter(async () => calls.push("worker"));
  return { calls, env, route };
}

test("routes /mcp paths to the MCPServer object", async () => {
  const { calls, env, route } = setup();
  await route(new Request("https://proxy.example/mcp"), env);
  await route(new Request("https://proxy.example/mcp/"), env);
  assert.deepEqual(calls, ["do:id:mcp", "do:id:mcp"]);
});

test("routes other paths to the per-request handler", async () => {
  const { calls, env, route } = setup();
  await route(new Request("https://proxy.example/mcpx"), env);
  await route(new Request("https://proxy.example/v1/models"), env);
  assert.deepEqual(calls, ["worker", "worker"]);
});
