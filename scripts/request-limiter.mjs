export function createRequestLimiter(handle, { maxActive = 5, maxWaiting = 10, waitMs = 10000 } = {}) {
  let active = 0;
  const waiting = [];

  function release() {
    const next = waiting.shift();
    if (next) {
      next(true);
    } else {
      active--;
    }
  }

  return async function limited(request, ...args) {
    if (active < maxActive) {
      active++;
    } else if (waiting.length < maxWaiting) {
      let queued;
      const ready = new Promise((resolve) => { queued = resolve; });
      waiting.push(queued);
      const timeout = setTimeout(() => {
        const index = waiting.indexOf(queued);
        if (index !== -1) {
          waiting.splice(index, 1);
          queued(false);
        }
      }, waitMs);
      const started = await ready;
      clearTimeout(timeout);
      if (!started) return busy();
    } else {
      return busy();
    }

    let released = false;
    const finish = () => {
      if (!released) {
        released = true;
        release();
      }
    };
    try {
      const response = await handle(request, ...args);
      if (!response.body) {
        finish();
        return response;
      }
      const reader = response.body.getReader();
      const body = new ReadableStream({
        async pull(controller) {
          try {
            const { value, done } = await reader.read();
            if (done) {
              controller.close();
              finish();
            } else {
              controller.enqueue(value);
            }
          } catch (error) {
            controller.error(error);
            finish();
          }
        },
        async cancel(reason) {
          try {
            await reader.cancel(reason);
          } finally {
            finish();
          }
        },
      });
      return new Response(body, response);
    } catch (error) {
      finish();
      throw error;
    }
  };
}

function busy() {
  return new Response(JSON.stringify({ error: "Proxy busy; retry shortly" }), {
    status: 503,
    headers: { "Content-Type": "application/json", "Retry-After": "1" },
  });
}
