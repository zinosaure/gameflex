import { apiRequest } from "@/service/api-client";
import { kernelRequest, kernelTarget } from "../../../libs/kernelChannel.js";
import { getStoredToken } from "../../../profile/auth/AuthContext.jsx";

// WebRTC Offer request

// WebRTC Offer response

// WebRTC Answer request

// WebRTC ICE Candidate request

// Remote Play Session info

// Latency statistics

// WebRTC config (includes TURN server config)

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
	startSession: async (hostId, username, host = null) => {
		const result = await kernelRequest(kernelTarget(host, getStoredToken()), "launch", {
			platform: "remoteplay",
			hostId,
			...(username ? { username } : {}),
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

	/**
	 * ICE servers for the BROWSER's own RTCPeerConnection (distinct from the
	 * Go backend's own PeerConnection, which reads the same config
	 * server-side via internal/iceservers — see that package's doc).
	 *
	 * Reads /streaming/webrtc/config, which returns the COMPUTED list rather
	 * than the stored settings: eturnal/Cloudflare credentials are minted
	 * per call and short-lived, so there's nothing the client could derive
	 * itself. This used to fetch the admin route (/api/v1/turn-config)
	 * directly, which no longer serves this — it's a POST behind a
	 * passphrase now, and it never returned usable credentials for the
	 * derived providers anyway.
	 *
	 * Reshaped into {turnServers: [{url, username, credential}]}, the shape
	 * this file's callers already expect, so none of them need to change.
	 */
	// `host` — `{origin, token}` quand le flux vit chez un autre serveur que
	// celui que la page vise (une vignette fédérée du tableau de bord).
	getTurnConfig: async (host = null) => {
		const response = await apiRequest("/remoteplay/streaming/webrtc/config", {
			method: "GET",
			...(host ? { host } : {}),
		});
		if (!response.success || !response.data?.iceServers) {
			return { success: false, data: { turnServers: [] } };
		}
		const turnServers = response.data.iceServers
			.filter((s) => s.urls && s.urls.length > 0)
			.map((s) => ({
				url: s.urls,
				username: s.username,
				credential: s.credential,
			}));
		return { success: true, data: { turnServers } };
	},
};
