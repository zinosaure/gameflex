import { apiRequest } from "../../../../libs/api-client";

// Device info type

// Bind device request type

// User device type

/**
 * PlayStation device service
 */
export const playStationService = {
	/**
	 * Discover PlayStation consoles on the local network
	 */
	discoverDevices: async (timeoutMs) => {
		const params = timeoutMs ? `?timeoutMs=${timeoutMs}` : "";
		return apiRequest(`/playstation/discover${params}`, {
			method: "GET",
		});
	},

	/**
	 * Bind a PS console to the current user
	 */
	bindDevice: async (data) => {
		return apiRequest("/playstation/bind", {
			method: "POST",
			body: JSON.stringify(data),
		});
	},

	// LE FLUX N'EST PLUS UN RÉGLAGE DE LA CONSOLE (le porteur, 2026-11-16 :
	// « C'est l'utilisateur qui choisit ») : `getDeviceSettings` et
	// `updateDeviceSettings` sont partis avec `/playstation/device-settings`.
	// Il est retenu dans le navigateur et part avec la connexion
	// (`streamingService.startSession`).
};
