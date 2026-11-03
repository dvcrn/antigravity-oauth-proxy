import { test } from "node:test";
import assert from "node:assert/strict";
import { createMCPRouter } from "./mcp-router.mjs";

function setup(status = 200) {
  const calls = [];
  const env = {
    MCP_SERVER: {
      idFromName: (name) => `id:${name}`,
      get: (id) => ({
        fetch: async (request) => {
          const { pathname } = new URL(request.url);
          calls.push(`do:${id}${pathname}`);
          if (pathname === "/__mcp_server/reset") throw new Error("aborted");
          return new Response(null, { status: 204 });
        },
      }),
    },
  };
  const route = createMCPRouter(async () => {
    calls.push("worker");
    return new Response(null, { status });
  });
  return { calls, env, route };
}

test("routes /mcp paths to the MCPServer object", async () => {
  const { calls, env, route } = setup();
  await route(new Request("https://proxy.example/mcp"), env);
  await route(new Request("https://proxy.example/mcp/"), env);
  assert.deepEqual(calls, ["do:id:mcp/mcp", "do:id:mcp/mcp/"]);
});

test("routes other paths to the per-request handler", async () => {
  const { calls, env, route } = setup();
  await route(new Request("https://proxy.example/mcpx"), env);
  await route(new Request("https://proxy.example/v1/models"), env);
  assert.deepEqual(calls, ["worker", "worker"]);
});

test("resets the MCP server after credentials are stored", async () => {
  const { calls, env, route } = setup();
  const response = await route(new Request("https://proxy.example/admin/tokens", { method: "POST" }), env);
  assert.equal(response.status, 200);
  assert.deepEqual(calls, ["worker", "do:id:mcp/__mcp_server/reset"]);
});

test("does not reset after a failed or read-only credentials request", async () => {
  const failed = setup(401);
  await failed.route(new Request("https://proxy.example/admin/auth/status", { method: "POST" }), failed.env);
  assert.deepEqual(failed.calls, ["worker"]);

  const readOnly = setup();
  await readOnly.route(new Request("https://proxy.example/admin/auth/status"), readOnly.env);
  assert.deepEqual(readOnly.calls, ["worker"]);
});
