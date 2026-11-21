import { useCallback, useEffect, useRef } from "react";
import {
	scanWebRTCReport,
	rateDeltas,
} from "@/gameflex/stream/libs/webrtc-stats.js";

// useConnectionStatsCollector — the Live Monitoring panel's 1s getStats()
// poll, extracted from use-streaming-connection.js (P7.4). Owns every ref
// the collection touches (rate-delta anchors, the 10-minute frame-count
// history, the streaming start clock) plus the interval that drives it —
// the caller only hands in the getRawStats source and the stats setter,
// and reads streamingStartTimeRef (stamped by handleConnectionStateChange /
// onFirstVideoTrack) and resetConnectionStats (disconnect) back out.
export const useConnectionStatsCollector = ({ screenRef, setConnectionStats, isStatsEnabled }) => {
	const statsIntervalRef = useRef(null);
	const isStatsEnabledRef = useRef(false);
	const previousStatsRef = useRef(null);
	const frameCountHistoryRef = useRef([]);
	const streamingStartTimeRef = useRef(null);

	const collectConnectionStats = useCallback(async () => {
		if (!isStatsEnabledRef.current) {
			return;
		}

		// getRawStats() — GatewayWebRTC's escape hatch for a caller
		// that needs getStats() itself rather than the already-summarized
		// shape its own onStatsUpdate reports (this panel wants more
		// fields — frameCountHistory, videoPacketsLost, streamingDurationMs
		// — than that shared shape carries).
		const statsReport = await screenRef.current?.getRawStats();
		if (!statsReport) {
			return;
		}

		try {
			// Shared getStats harvesting (see gameflex/stream/libs/webrtc-stats.js)
			// — the same report scan and rate math as streamex's performance
			// panel, so the two consumers can't drift apart.
			const harvested = scanWebRTCReport(statsReport);
			const totalInboundBytes = harvested.inboundBytes;
			const totalOutboundBytes = harvested.outboundBytes;
			const videoInboundBytes = harvested.videoInboundBytes;
			const {
				frameWidth,
				frameHeight,
				framesDecoded,
				latencyMs,
				videoPacketsLost,
				videoJitterMs,
			} = harvested;

			// Calculate fps from WebRTC stats
			let fps = null;
			const now = performance.now();
			const previous = previousStatsRef.current;
			let rates = null;
			if (previous) {
				rates = rateDeltas(
					{
						timestamp: previous.timestamp,
						inboundBytes: previous.bytesReceived,
						outboundBytes: previous.bytesSent,
						videoInboundBytes: previous.videoBytesReceived,
						framesDecoded: previous.framesDecoded ?? 0,
					},
					now,
					harvested,
				);
				if (rates) {
					fps = rates.fps;
				}
			}

			const currentTime = Date.now();
			const tenMinutesAgo = currentTime - 10 * 60 * 1000;

			frameCountHistoryRef.current = frameCountHistoryRef.current.filter(
				(point) => point.timestamp >= tenMinutesAgo,
			);

			if (fps !== null && Number.isFinite(fps) && fps >= 0) {
				frameCountHistoryRef.current.push({
					timestamp: currentTime,
					frameCount: fps,
				});
			}

			// Calculate streaming duration
			let streamingDurationMs = null;
			if (streamingStartTimeRef.current !== null) {
				streamingDurationMs = Date.now() - streamingStartTimeRef.current;
			}

			if (!previous) {
				previousStatsRef.current = {
					timestamp: now,
					bytesReceived: totalInboundBytes,
					bytesSent: totalOutboundBytes,
					videoBytesReceived: videoInboundBytes,
					framesDecoded: framesDecoded ?? null,
				};

				setConnectionStats((prev) => ({
					downloadKbps: prev?.downloadKbps ?? null,
					uploadKbps: prev?.uploadKbps ?? null,
					videoBitrateKbps: prev?.videoBitrateKbps ?? null,
					resolution:
						frameWidth !== null && frameHeight !== null
							? { width: frameWidth, height: frameHeight }
							: (prev?.resolution ?? null),
					latencyMs: latencyMs ?? prev?.latencyMs ?? null,
					fps: fps ?? prev?.fps ?? null,
					videoPacketsLost: videoPacketsLost ?? prev?.videoPacketsLost ?? null,
					videoJitterMs: videoJitterMs ?? prev?.videoJitterMs ?? null,
					streamingDurationMs: streamingDurationMs,
					frameCountHistory: [...frameCountHistoryRef.current],
					debug: prev?.debug ?? null,
				}));

				return;
			}

			if (rates === null) {
				return;
			}

			const downloadKbps = rates.downloadKbps;
			const uploadKbps = rates.uploadKbps;
			const videoBitrateKbps = rates.videoBitrateKbps;

			previousStatsRef.current = {
				timestamp: now,
				bytesReceived: totalInboundBytes,
				bytesSent: totalOutboundBytes,
				videoBytesReceived: videoInboundBytes,
				framesDecoded: framesDecoded ?? previous.framesDecoded ?? 0,
			};

			setConnectionStats((prev) => ({
				downloadKbps: Number.isFinite(downloadKbps)
					? downloadKbps
					: (prev?.downloadKbps ?? null),
				uploadKbps: Number.isFinite(uploadKbps)
					? uploadKbps
					: (prev?.uploadKbps ?? null),
				videoBitrateKbps: Number.isFinite(videoBitrateKbps)
					? videoBitrateKbps
					: (prev?.videoBitrateKbps ?? null),
				resolution:
					frameWidth !== null && frameHeight !== null
						? { width: frameWidth, height: frameHeight }
						: (prev?.resolution ?? null),
				latencyMs: latencyMs ?? prev?.latencyMs ?? null,
				fps: fps ?? prev?.fps ?? null,
				videoPacketsLost: videoPacketsLost ?? prev?.videoPacketsLost ?? null,
				videoJitterMs: videoJitterMs ?? prev?.videoJitterMs ?? null,
				streamingDurationMs: streamingDurationMs,
				frameCountHistory: [...frameCountHistoryRef.current],
				debug: prev?.debug ?? null,
			}));
		} catch (error) {
			console.warn("Failed to fetch WebRTC stats:", error);
		}
	}, [screenRef, setConnectionStats]);

	useEffect(() => {
		isStatsEnabledRef.current = isStatsEnabled;

		if (!isStatsEnabled) {
			if (statsIntervalRef.current !== null) {
				window.clearInterval(statsIntervalRef.current);
				statsIntervalRef.current = null;
			}
			previousStatsRef.current = null;
			return;
		}

		const tick = () => {
			collectConnectionStats().catch((error) => {
				console.warn("Failed to update WebRTC stats:", error);
			});
		};

		tick();
		statsIntervalRef.current = window.setInterval(tick, 1000);

		return () => {
			if (statsIntervalRef.current !== null) {
				window.clearInterval(statsIntervalRef.current);
				statsIntervalRef.current = null;
			}
		};
	}, [collectConnectionStats, isStatsEnabled]);

	// resetConnectionStats — disconnect()'s teardown: clears the deltas so a
	// reconnect starts fresh instead of computing rates across the gap.
	const resetConnectionStats = useCallback(() => {
		previousStatsRef.current = null;
		frameCountHistoryRef.current = [];
		streamingStartTimeRef.current = null;
	}, []);

	return { streamingStartTimeRef, resetConnectionStats };
};
