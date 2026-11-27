import { test } from "node:test";
import assert from "node:assert/strict";
import { fitInside, ICON_SIZE } from "./markIcon.ts";

test("a picture is drawn to fit 64 pixels a side, its shape kept and never enlarged", () => {
  assert.equal(ICON_SIZE, 64);
  assert.deepEqual(fitInside(512, 512), { width: 64, height: 64 });
  assert.deepEqual(fitInside(1024, 512), { width: 64, height: 32 });
  assert.deepEqual(fitInside(300, 1200), { width: 16, height: 64 });
  assert.deepEqual(fitInside(20, 10), { width: 20, height: 10 });
  // A long thin one keeps at least a pixel.
  assert.deepEqual(fitInside(5000, 10), { width: 64, height: 1 });
  // An SVG with no size of its own fills the square.
  assert.deepEqual(fitInside(0, 0), { width: 64, height: 64 });
});
