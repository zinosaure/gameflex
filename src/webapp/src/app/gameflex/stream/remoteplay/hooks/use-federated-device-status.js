import { useEffect } from "react";
import { listFederatedDevices } from "@/gameflex/api/remoteplay";

// LES CONSOLES FÉDÉRÉES N'ONT PAS D'ÉTAT EN DIRECT (2026-10-25).
//
// Le WebSocket d'état (use-device-status.js) ne connaît que les consoles
// d'ici. Une console prêtée par un serveur fédéré restait donc sur « Veille »
// après « Allumer », carte et panneau, jusqu'à ce qu'on quitte la page (le
// porteur, depuis le téléphone). Son état se relit par la liste fédérée, que
// notre kernel va chercher chez chaque serveur — au rythme du sondage que le
// service d'ici fait de ses propres consoles, et seulement quand la page se
// voit.
const FEDERATED_POLL_MS = 5_000;

/**
 * Relit les consoles fédérées toutes les 5 s tant que `enabled`, et passe la
 * liste à `onDevices` — la même forme que listFederatedDevices().data.
 */
export function useFederatedDeviceStatus({ enabled, onDevices }) {
	useEffect(() => {
		if (!enabled) {
			return undefined;
		}

		let cancelled = false;
		const timer = setInterval(async () => {
			if (document.hidden) {
				return;
			}
			const result = await listFederatedDevices();
			if (!cancelled && result.ok) {
				onDevices(result.data || []);
			}
		}, FEDERATED_POLL_MS);

		return () => {
			cancelled = true;
			clearInterval(timer);
		};
	}, [enabled, onDevices]);
}
