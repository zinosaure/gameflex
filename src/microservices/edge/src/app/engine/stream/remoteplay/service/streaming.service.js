import { apiRequest } from "../../../../libs/api-client";
import { kernelRequest, kernelTarget } from "../../../../libs/kernelChannel.js";
import { getStoredToken } from "../../../../libs/sharedHost.js";

// WebRTC Offer request

// WebRTC Offer response

// WebRTC Answer request

// WebRTC ICE Candidate request

// Remote Play Session info

// Latency statistics

/**
 * Streaming service
 */
export const streamingService = {
	/**
	 * Create WebRTC Offer
	 */
	createOffer: async (payload) => {
		const hasPayload = payload && Object.keys(payload).length > 0;
		return apiRequest("/remoteplay/webrtc/offer", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: hasPayload ? JSON.stringify(payload) : undefined,
		});
	},

	/**
	 * Send WebRTC Answer
	 */
	/**
	 * Create or retrieve a Remote Play Session from Host ID. username is
	 * optional (falls back to "anonymous" server-side) — sent so the Go
	 * backend can mirror this session into its cross-platform sessions
	 * registry with a real Initiator (see internal/sessions/pool.go).
	 */
	// LANCER UNE CONSOLE — la commande `launch` du centre de messages, comme
	// les deux autres plateformes. Le kernel lance, puis demande au runtime la
	// poignée de main Takion sur le port obtenu — les clés d'appairage de la
	// console ne traversent jamais le kernel (voir `launch_remoteplay.go`).
	//
	// La réponse garde l'enveloppe d'avant (`data.sessionId`), et un refus
	// LÈVE une erreur portant le message du kernel, comme le faisait
	// `apiRequest` : `RemotePlay.jsx` y reconnaît le temps de repos de la
	// console (« init/ctrl rejected »).
	//
	// `settings` — le flux de la console dans ce navigateur (Platforms.jsx,
	// streamOf), son débit « Auto » déjà mesuré (withNetworkBitrate), dans le
	// vocabulaire et les champs du lancement d'un jeu (`launchSession`,
	// spawnSessions.js) : le kernel le revalide, puis le runtime le traduit
	// pour la console. Absent : le défaut de toutes les consoles, 720p à 60.
	startSession: async (hostId, username, host = null, settings = null) => {
		const result = await kernelRequest(kernelTarget(host, getStoredToken()), "launch", {
			platform: "remoteplay",
			hostId,
			...(username ? { username } : {}),
			...(settings?.resolution ? { resolution: settings.resolution } : {}),
			...(settings?.frameRate ? { fps: Number(settings.frameRate) } : {}),
			...(settings?.bitrate && settings.bitrate !== "auto"
				? { bitrate: Number(settings.bitrate) }
				: {}),
		});
		if (!result.ok) {
			// La réponse entière voyage avec l'erreur : un refus « une autre
			// partie est là » porte de quoi proposer un choix (LaunchConflict).
			throw Object.assign(new Error(result.errorMessage || `Request failed: ${result.status}`), { status: result.status, result });
		}

		return result;
	},

	/**
	 * Connect WebRTC Session to Remote Play Session
	 */
	connectToRemotePlaySession: async (webrtcSessionId, remotePlaySessionId) => {
		return apiRequest(
			`/remoteplay/webrtc/connect/${webrtcSessionId}/${remotePlaySessionId}`,
			{
				method: "POST",
			},
		);
	},

	/**
	 * Delete WebRTC Session
	 */
	deleteSession: async (sessionId) => {
		return apiRequest(`/remoteplay/webrtc/session/${sessionId}`, {
			method: "DELETE",
		});
	},

	/**
	 * Request a keyframe
	 */
	requestKeyframe: async (sessionId) => {
		return apiRequest(
			`/remoteplay/webrtc/session/${encodeURIComponent(sessionId)}/keyframe`,
			{
				method: "POST",
			},
		);
	},
};
