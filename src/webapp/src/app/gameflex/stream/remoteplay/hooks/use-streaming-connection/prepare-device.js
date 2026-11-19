import { useCallback } from "react";
import { playStationService } from "@/gameflex/stream/remoteplay/service/playstation.service";
import { debugTrace } from "@/gameflex/shared/utils/log-store";

// prepareDevice — the PS5 discovery/wake/readiness gate connect() runs
// before the session handshake (getMyDevices → discover/wake → poll ready,
// 30 s max). Extracted from use-streaming-connection.js (P7.4): it touches
// no hook state — only the device service and the caller's own status
// setter — so it lives as a plain callback factory rather than a sub-hook
// owning refs, same pattern as stick-input-state's self-contained shape.
export const usePrepareDevice = ({ hostId, t, setConnectionState }) => {
	return useCallback(async () => {
		if (!hostId) {
			return false;
		}

		try {
			setConnectionState(t("streaming.connection.state.fetchingDevice"));
			const devicesResponse = await playStationService.getMyDevices();
			if (!devicesResponse.success || !devicesResponse.result) {
				throw new Error(t("streaming.connection.errors.fetchDeviceFailed"));
			}

			const device = devicesResponse.result.find((d) => d.hostId === hostId);
			if (!device) {
				throw new Error(t("streaming.connection.errors.deviceNotFound"));
			}

			if (!device.ipAddress) {
				throw new Error(t("streaming.connection.errors.ipNotSet"));
			}

			const deviceIp = device.ipAddress;

			setConnectionState(t("streaming.connection.state.checkingStatus"));
			let firstStatusCheck = await playStationService
				.discoverDevice(deviceIp, 5000)
				.catch(() => {
					console.warn(
						"Initial status query failed, will continue querying in the wait loop...",
					);
					return { success: false, result: null };
				});

			if (!firstStatusCheck.success || !firstStatusCheck.result) {
				console.warn("Initial status query failed, retrying once...");
				await new Promise((resolve) => setTimeout(resolve, 1000));
				firstStatusCheck = await playStationService
					.discoverDevice(deviceIp, 5000)
					.catch(() => {
						console.warn(
							"Initial status query retry also failed, will continue querying in the wait loop...",
						);
						return { success: false, result: null };
					});
			}

			let needWaitForReady = false;
			if (firstStatusCheck.success && firstStatusCheck.result) {
				const deviceStatus =
					firstStatusCheck.result.status?.toUpperCase() || "";
				debugTrace("Device current status:", deviceStatus);

				if (deviceStatus.includes("STANDBY")) {
					setConnectionState(t("streaming.connection.state.wakingUp"));

					const wakeResponse = await playStationService.wakeUpConsole(hostId);
					if (!wakeResponse.success || !wakeResponse.result) {
						throw new Error(t("streaming.connection.errors.wakeDeviceFailed"));
					}

					debugTrace(
						"✅ Device wake command sent, waiting for device to be ready...",
					);
					needWaitForReady = true;
				} else if (
					deviceStatus === "OK" ||
					deviceStatus.includes("READY") ||
					deviceStatus.includes("AVAILABLE")
				) {
					debugTrace("✅ Device is ready, status:", deviceStatus);
					return true;
				} else {
					debugTrace(
						"⚠️ Device status:",
						deviceStatus,
						", waiting for device to be ready...",
					);
					needWaitForReady = true;
				}
			} else {
				debugTrace(
					"⚠️ Initial status query failed, waiting for device to be ready...",
				);
				needWaitForReady = true;
			}

			if (needWaitForReady) {
				setConnectionState(t("streaming.connection.state.waitingReady"));
				const timeout = 30000;
				const checkInterval = 1000;
				const startTime = Date.now();

				debugTrace("🔄 Starting active device status polling...");

				while (Date.now() - startTime < timeout) {
					try {
						const elapsed = Math.floor((Date.now() - startTime) / 1000);
						debugTrace(`📡 Polling device status... (${elapsed}s)`);
						const statusResponse = await Promise.race([
							playStationService.discoverDevice(deviceIp, 5000),
							new Promise((_, reject) =>
								setTimeout(() => reject(new Error("Query timeout")), 6000),
							),
						]).catch((error) => {
							debugTrace(
								`⚠️ Device status query timed out or failed (${elapsed}s):`,
								error,
							);
							return { success: false, result: null };
						});

						if (statusResponse.success && statusResponse.result) {
							const currentStatus =
								statusResponse.result.status?.toUpperCase() || "";
							debugTrace(
								`✅ Device status check (${elapsed}s):`,
								currentStatus,
							);

							if (
								currentStatus === "OK" ||
								currentStatus.includes("READY") ||
								currentStatus.includes("AVAILABLE")
							) {
								debugTrace("✅ Device is ready, status:", currentStatus);
								return true;
							} else {
								debugTrace(
									`⏳ Device not yet ready, current status: ${currentStatus}, continuing to wait...`,
								);
							}
						} else {
							debugTrace(
								`⚠️ Device status query failed (${elapsed}s), retrying...`,
							);
						}
					} catch (queryError) {
						const elapsed = Math.floor((Date.now() - startTime) / 1000);
						debugTrace(
							`⚠️ Device status query error (${elapsed}s):`,
							queryError,
							", retrying...",
						);
					}

					const elapsed = Math.floor((Date.now() - startTime) / 1000);
					setConnectionState(
						t("streaming.connection.state.waitingReadyWithTime", {
							seconds: elapsed,
						}),
					);

					if (Date.now() - startTime >= timeout) {
						break;
					}

					await new Promise((resolve) => setTimeout(resolve, checkInterval));
				}

				const finalElapsed = Math.floor((Date.now() - startTime) / 1000);
				console.error(`❌ Device ready timeout (${finalElapsed}s)`);
				throw new Error(
					t("streaming.connection.errors.deviceReadyTimeout", {
						seconds: finalElapsed,
					}),
				);
			}

			return false;
		} catch (error) {
			console.error("Device preparation failed:", error);
			const errorMessage =
				error instanceof Error
					? error.message
					: t("streaming.connection.errors.unknown");
			const normalizedErrorMessage = errorMessage.toLowerCase();
			// Update connection state; no longer show a popup toast
			if (
				normalizedErrorMessage.includes("timeout") ||
				errorMessage.includes("timeout")
			) {
				setConnectionState(t("streaming.connection.state.failed"));
				console.warn("Device preparation timed out:", errorMessage);
			} else {
				console.warn(
					"Device preparation encountered an error but continuing to wait:",
					errorMessage,
				);
			}
			return false;
		}
	}, [hostId, t, setConnectionState]);
};
