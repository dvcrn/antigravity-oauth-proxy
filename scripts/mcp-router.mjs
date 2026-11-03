const mcpPath = /^\/mcp(?:\/|$)/;

// Sends /mcp to the single MCPServer Durable Object and everything else to the
// per-request Go handler.
export function createMCPRouter(handle) {
  return function route(request, env, ...args) {
    if (mcpPath.test(new URL(request.url).pathname)) {
      const namespace = env.MCP_SERVER;
      return namespace.get(namespace.idFromName("mcp")).fetch(request);
    }
    return handle(request, env, ...args);
  };
}
