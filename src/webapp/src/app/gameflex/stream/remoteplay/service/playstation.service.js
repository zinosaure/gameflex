import { apiRequest } from "@/service/api-client";

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
	 * Discover a PlayStation console at a specific IP
	 */
	discoverDevice: async (hostIp, timeoutMs) => {
		const params = timeoutMs ? `?timeoutMs=${timeoutMs}` : "";
		return apiRequest(`/playstation/discover/${hostIp}${params}`, {
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

	/**
	 * Get the list of devices bound to the current user
	 */
	getMyDevices: async () => {
		return apiRequest("/playstation/my-devices", {
			method: "GET",
		});
	},

	/**
	 * Wake up console from standby
	 */
	wakeUpConsole: async (userDeviceId) => {
		return apiRequest(`/playstation/wake/${encodeURIComponent(userDeviceId)}`, {
			method: "POST",
		});
	},

	/**
	 * Get device streaming settings
	 */
	getDeviceSettings: async (deviceId, host = null) => {
		return apiRequest(`/playstation/device-settings/${deviceId}`, {
			method: "GET",
			host,
		});
	},

	/**
	 * Update device streaming settings
	 */
	updateDeviceSettings: async (deviceId, data, host = null) => {
		return apiRequest(`/playstation/device-settings/${deviceId}`, {
			method: "POST",
			body: JSON.stringify(data),
			host,
		});
	},
};
