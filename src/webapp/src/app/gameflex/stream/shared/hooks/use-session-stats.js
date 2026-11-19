import { useEffect, useState } from "react";
import { can } from "../../../libs/capabilities.js";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../../libs/kernelChannel.js";
import { homeOrigin, streamAuthToken } from "../../../libs/serverBase.js";

// The performance-panel plumbing every platform's useXInput hook rebuilt on
// its own: a statsOpen toggle, syncing it into the connection hook's own
// getStats() sampling (setStatsMonitoring), and the server telemetry, which
// only flows while the panel is open. The only real per-platform variance is
// the topic segment; every kernel (server and phone) pushes the same
// `sessionstatus.Status`, which the caller reads via `serverStatus?.data`.
//
// PUSHED, NEVER POLLED. The panel subscribes to `status/<platform>/<session>`
// on the kernel's message centre (libs/kernelChannel.js) — the same socket as
// the session list and the launch steps — and the kernel pushes the snapshot
// at the requested pace (`every`, bounded server-side) until the panel
// closes. A missed reading on the server side simply sends nothing: the
// panel keeps the previous snapshot.
//
// renderContent — (serverStatus) => ReactNode, called on every render (not
// memoized) so it always sees the latest connectionStats/serverStatus —
// same as the plain object literal every Player.jsx built inline before.
//
// devicePower — LE TÉLÉPHONE QU'ON TIENT, dans l'application seulement
// (capacité `devicePower`) : sa température et sa batterie, que notre propre
// kernel donne (sujet `power/…`) où que tourne la partie. Le porteur, le
// 2026-10-29 : « les joueurs préfèrent avoir l'info de leur téléphone aussi
// (et en premier) » — et, dans un navigateur, ni les unes ni les pastilles.
//
// powerOnly — LES PASTILLES (2026-10-28) : à côté du bouton du menu, elles
// suivent le téléphone même panneau fermé, au pas de POWER_INTERVAL_MS — le
// seul abonnement qui vit sans panneau (setStatsMonitoring suit le panneau
// seul).
const POWER_INTERVAL_MS = 10000;

export function useSessionStats({ platform, sessionId, startedAt = null, token, setStatsMonitoring, intervalMs = 2000, open = false, powerOnly = false }) {
	const follow = open;
	const followDevice = can("devicePower") && (open || powerOnly);
	const [serverStatus, setServerStatus] = useState(null);
	const [devicePower, setDevicePower] = useState(null);

	// Mirrors the caller's own open state into the connection hook's own
	// getStats polling (same pattern every platform used) — the panel's
	// open/closed state itself now lives with the caller (Streaming.jsx's
	// FloatingPanel switch is the single source of truth), not here.
	useEffect(() => {
		setStatsMonitoring(open);
	}, [open, setStatsMonitoring]);

	useEffect(() => {
		if (!follow || !sessionId) {
			setServerStatus(null);
		}
	}, [follow, sessionId]);

	useEffect(() => {
		if (!followDevice || !sessionId) {
			setDevicePower(null);
		}
	}, [followDevice, sessionId]);

	// LE JETON VA AVEC L'ADRESSE. `kernelTarget(null, …)` vise le kernel de
	// la page (`serverOrigin`, qui suit la surcharge d'une page fédérée) :
	// c'est donc le jeton de NOTRE COMPTE LÀ-BAS qu'il faut présenter. Le
	// nôtre y était refusé : panneau de performances vide sur une partie
	// fédérée, et un canal qui réessayait sans fin tant qu'il restait ouvert
	// (relevé le 2026-10-09).
	const hostToken = streamAuthToken() || token;
	const channel = useKernelChannel(follow && sessionId && kernelReachable(hostToken) ? kernelTarget(null, hostToken) : null);

	// `{data}` — the envelope the panel already unwraps, kept so the three
	// platform hooks read the snapshot exactly as before.
	useKernelTopic(
		channel,
		follow && sessionId ? `status/${platform}/${sessionId}` : null,
		{ every: intervalMs },
		(status) => setServerStatus({ success: true, data: status }),
	);

	// CHEZ NOUS, avec NOTRE jeton — pas celui de l'hôte (`homeOrigin`, comme
	// SessionPool). La clé dit la partie, son heure de départ comprise : le
	// kernel garde le départ de la mesure, et un identifiant réutilisé le
	// lendemain repart du sien.
	const homeChannel = useKernelChannel(followDevice && sessionId && kernelReachable(token)
		? { origin: homeOrigin(), token: token || "" }
		: null);
	useKernelTopic(
		homeChannel,
		followDevice && sessionId ? `power/${platform}/${sessionId}@${startedAt || ""}` : null,
		{ every: open ? intervalMs : POWER_INTERVAL_MS },
		(power) => setDevicePower(power),
	);

	return { serverStatus, devicePower };
}
