import { useEffect, useRef } from "react";
import { remoteplayInputService } from "@/gameflex/stream/remoteplay/service/input.service";

/**
 * Live device status via the unified input WebSocket (see
 * stream/remoteplay/service/input.service.js) — replaces the previous 5s HTTP
 * poll of GET /playstation/my-devices. External contract unchanged:
 * onDevicesUpdate fires once with the initial snapshot, onStatusUpdate
 * fires on any subsequent status/hasActiveSession change per device.
 *
 * `enabled` (défaut vrai) : un hôte qui n'ouvre pas de session n'a pas ce
 * service, donc pas ce WebSocket. Sans ce drapeau, l'application passait son
 * temps à retenter une connexion qui n'existe pas, et la console du
 * navigateur noyait les vraies pannes sous ses avertissements (chantier 9).
 */
export function useDeviceStatus({ onDevicesUpdate, onStatusUpdate, enabled = true }) {
	const previousDevicesRef = useRef([]);
	const hasReceivedFirstUpdateRef = useRef(false);

	useEffect(() => {
		if (!enabled) {
			return undefined;
		}

		let cancelled = false;

		const handleDevices = (devices) => {
			if (cancelled || !devices) return;
			const prevDevices = previousDevicesRef.current;

			if (!hasReceivedFirstUpdateRef.current) {
				hasReceivedFirstUpdateRef.current = true;
				onDevicesUpdate(devices);
			} else {
				const hasChanges = devices.some((d) => {
					const prev = prevDevices.find((p) => p.hostId === d.hostId);
					return (
						!prev ||
						prev.status !== d.status ||
						prev.hasActiveSession !== d.hasActiveSession
					);
				});
				if (hasChanges) {
					onStatusUpdate(devices);
				}
			}
			previousDevicesRef.current = devices;
		};

		const unsubscribe = remoteplayInputService.onDevices(handleDevices);

		// The service pushes an immediate snapshot on subscribe, so no
		// separate initial HTTP fetch is needed — and the singleton means
		// every consumer shares one underlying WebSocket.
		remoteplayInputService.subscribeDevices().catch((error) => {
			console.warn("device-status connect failed:", error);
		});

		return () => {
			cancelled = true;
			unsubscribe();
			// Deliberately not calling disconnect() here — the connection
			// is a shared singleton and stays open across unmounts so
			// navigating back to this page doesn't pay a reconnect cost.
		};
	}, [onDevicesUpdate, onStatusUpdate, enabled]);
}
