const assert = require("assert");
const {
    fallbackRateDeltas,
    nextFallbackCounters,
    parseFallbackControl,
    smoothFallbackLatency,
} = require("../../src/webapp/src/app/gameflex/stream/libs/webrtc-stats.js");

const first = nextFallbackCounters(1000, {
    downloadBytes: 1000,
    uploadBytes: 100,
    videoBytes: 900,
    decodedFrames: 10,
});
const rates = fallbackRateDeltas(first, 3000, {
    downloadBytes: 5000,
    uploadBytes: 1100,
    videoBytes: 3900,
    decodedFrames: 34,
});

assert.strictEqual(rates.downloadKbps, 16);
assert.strictEqual(rates.uploadKbps, 4);
assert.strictEqual(rates.videoBitrateKbps, 12);
assert.strictEqual(rates.fps, 12);
assert.strictEqual(fallbackRateDeltas(first, 1000, {}), null);
assert.deepStrictEqual(
    parseFallbackControl('{"type":"fallback-ping","sentAt":1234}'),
    { type: "fallback-ping", sentAt: 1234 },
);
assert.deepStrictEqual(parseFallbackControl("fallback-ping"), { type: "fallback-ping" });
assert.strictEqual(smoothFallbackLatency(null, 40), 40);
assert.strictEqual(smoothFallbackLatency(40, 50), 42);
assert.strictEqual(smoothFallbackLatency(42, Number.NaN), 42);

console.log("Fallback HTTPS : débits/FPS réels et protocole ping/latence validés.");
