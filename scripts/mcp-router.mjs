const mcpPath = /^\/mcp(?:\/|$)/;
const resetPath = "/__mcp_server/reset";
// Admin routes that store new OAuth credentials. The MCP server discovers the
// project ID once at startup, so it restarts after any of them succeed.
const credentialRoutes = new Set(["/admin/credentials", "/admin/tokens", "/admin/auth/status"]);

function mcpServer(env) {
  const namespace = env.MCP_SERVER;
  return namespace.get(namespace.idFromName("mcp"));
}

// Sends /mcp to the single MCPServer Durable Object and everything else to the
// per-request Go handler.
export function createMCPRouter(handle) {
  return async function route(request, env, ...args) {
    const { pathname } = new URL(request.url);
    if (mcpPath.test(pathname)) {
      return mcpServer(env).fetch(request);
    }

    const response = await handle(request, env, ...args);
    if (request.method === "POST" && response.ok && credentialRoutes.has(pathname)) {
      try {
        await mcpServer(env).fetch(new Request(`https://mcp-server${resetPath}`, { method: "POST" }));
      } catch {
        // abort() rejects the reset call by design.
      }
    }
    return response;
  };
}

export const mcpServerResetPath = resetPath;
