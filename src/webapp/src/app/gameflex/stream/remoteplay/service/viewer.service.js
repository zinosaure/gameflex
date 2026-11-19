import { apiRequest } from "@/service/api-client";

/**
 * Viewer service — watch-only viewers of an already-running streaming
 * session (someone else's console). The viewer itself attaches to
 * gatewaywebrtc; remoteplay only confirms the stream is there. No controller
 * feedback, no Takion/session lifecycle calls — the owner's session is never
 * touched.
 */
export const viewerService = {
	/**
	 * Ask remoteplay to confirm a watch-only viewer for an active session —
	 * the reply names the gateway stream to attach to.
	 */
	createOffer: async (sessionId, codec) => {
		return apiRequest("/remoteplay/webrtc/viewer/offer", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ sessionId, codec }),
		});
	},
};
