import "../build/wasm_exec.js";
import { createRuntimeContext, loadModule } from "../build/runtime.mjs";

// MCPServer hosts one long-lived Go instance so in-memory MCP state, such as
// async jobs, is shared by every /mcp request. Pending upstream fetches keep the
// object alive while background jobs run.
export class MCPServer {
  constructor(ctx, env) {
    this.ctx = ctx;
    this.env = env;
    this.binding = undefined;
  }

  async fetch(request) {
    this.binding ??= startGo(this.env, this.ctx, () => {
      this.binding = undefined;
    });
    let binding;
    try {
      binding = await this.binding;
    } catch (error) {
      this.binding = undefined;
      throw error;
    }
    return binding.handleRequest(request);
  }
}

async function startGo(env, ctx, onExit) {
  const mod = await loadModule();
  const go = new Go();
  const binding = {};
  let ready;
  const readyPromise = new Promise((resolve) => {
    ready = resolve;
  });
  const instance = new WebAssembly.Instance(mod, {
    ...go.importObject,
    workers: { ready: () => ready() },
  });
  const exited = go.run(instance, { ...createRuntimeContext({ env, ctx, binding }), persistent: true });
  exited.finally(onExit);
  await Promise.race([
    readyPromise,
    exited.then(() => {
      throw new Error("Go program exited before it was ready");
    }),
  ]);
  return binding;
}
