// Model discovery is public and small; cache it per isolate so repeated picker
// lookups do not each initialize a Go/WASM instance.
export function createModelCache(handle, { ttlMs = 60000 } = {}) {
  const cache = new Map();
  const pending = new Map();

  return async function cached(request, ...args) {
    const url = new URL(request.url);
    if (request.method !== "GET" || url.search || !/^\/v1\/models(?:\/[^/]+)?\/?$/.test(url.pathname)) {
      return handle(request, ...args);
    }

    const key = url.pathname;
    const entry = cache.get(key);
    if (entry && entry.expires > Date.now()) {
      return new Response(entry.body, { status: 200, headers: entry.headers });
    }

    if (pending.has(key)) {
      const shared = await pending.get(key);
      if (shared) return new Response(shared.body, { status: 200, headers: shared.headers });
      return handle(request, ...args);
    }

    const load = (async () => {
      const response = await handle(request, ...args);
      if (response.status !== 200 || !response.headers.get("content-type")?.includes("application/json")) {
        return { response };
      }
      const body = await response.text();
      const record = {
        body,
        headers: { "Content-Type": "application/json" },
        expires: Date.now() + ttlMs,
      };
      cache.set(key, record);
      return { record };
    })();
    pending.set(key, load.then((result) => result.record ?? null, () => null));
    try {
      const result = await load;
      if (result.response) return result.response;
      return new Response(result.record.body, { status: 200, headers: result.record.headers });
    } finally {
      pending.delete(key);
    }
  };
}
