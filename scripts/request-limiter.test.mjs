import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequestLimiter } from "./request-limiter.mjs";

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

test("bounds running and waiting requests, then admits the next one", async () => {
  const gates = [];
  const limited = createRequestLimiter(async () => {
    await new Promise((resolve) => gates.push(resolve));
    return new Response(null, { status: 204 });
  }, { maxActive: 1, maxWaiting: 1 });

  const first = limited();
  const second = limited();
  assert.equal((await limited()).status, 503);
  assert.equal(gates.length, 1);
  gates.shift()();
  assert.equal((await first).status, 204);
  await tick();
  assert.equal(gates.length, 1);
  gates.shift()();
  assert.equal((await second).status, 204);
});

test("times out queued requests without starting them", async () => {
  let finish;
  let started = 0;
  const limited = createRequestLimiter(async () => {
    started++;
    await new Promise((resolve) => { finish = resolve; });
    return new Response(null);
  }, { maxActive: 1, maxWaiting: 1, waitMs: 10 });

  const first = limited();
  assert.equal((await limited()).status, 503);
  assert.equal(started, 1);
  finish();
  await first;
  assert.equal(started, 1);
});

test("holds a slot until a streamed body is consumed or cancelled", async () => {
  let started = 0;
  const limited = createRequestLimiter(async () => {
    started++;
    return new Response(new ReadableStream({ pull() {} }));
  }, { maxActive: 1, maxWaiting: 0 });

  const first = await limited();
  assert.equal((await limited()).status, 503);
  await first.body.cancel();
  const second = await limited();
  assert.equal(started, 2);
  await second.body.cancel();
});

test("releases a slot when the handler throws", async () => {
  const limited = createRequestLimiter(async () => { throw new Error("failed"); }, { maxActive: 1, maxWaiting: 0 });
  await assert.rejects(limited(), /failed/);
  await assert.rejects(limited(), /failed/);
});
