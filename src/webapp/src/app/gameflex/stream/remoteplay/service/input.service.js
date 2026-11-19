// stream/remoteplay/service/input.service.js — remote-play's input WebSocket, the SignalR replacement on the frontend.
//
// ONE JSON WebSocket per browser role against the remote-play service's
// /api/v1/remoteplay/input/ws (see internal/api/input_ws.go): bound at connect
// time via ?sessionId= (+ ?iceKey= — a viewer's own id — for a watcher's bind,
// which never takes the pad), fire-and-forget input, server pushes for
// rumble/health/control events. Replaces controller.service.js, streaming-hub.service.js and
// device-status-hub.service.js with a single transport.
//
// The public surface is the union of the three replaced services, so the
// four consumers (use-streaming-connection,
// use-device-status, Streaming.jsx) only change their import line.

import { getStoredToken } from "../../../profile/auth/AuthContext.jsx";
import { onGameInputSuspension } from "../../../libs/gameInput.js";
import { PS5_BUTTON_MAP } from "./gamepad.service.js";

// Tous les boutons qu'une entrée peut tenir — ceux de la manette, que le
// clavier et le pad tactile nomment de la même façon (voir setSuspended).
const RELEASABLE_BUTTONS = [...new Set(Object.values(PS5_BUTTON_MAP))];
import { streamAuthToken } from "../../../libs/serverBase.js";
import { wsUrl } from "../../../libs/serverBase.js";

const RECONNECT_MIN_DELAY_MS = 1000;
const RECONNECT_MAX_DELAY_MS = 30000;
const REQUEST_TIMEOUT_MS = 5000;

function debugTrace(...args) {
	if (typeof window !== "undefined" && window.__REMOTE_INPUT_DEBUG__) {
		console.debug("[remoteplay-input]", ...args);
	}
}

// decodeGatewayText — le canal de données livre du BINAIRE (voir
// dispatchGatewayMessage). On accepte les trois formes qu'un navigateur peut
// rendre : la chaîne, l'ArrayBuffer, et la vue typée. Le Blob est exclu
// exprès — il ne se lit qu'en asynchrone, et aucun navigateur ne le rend ici
// tant que `binaryType` vaut « arraybuffer », le défaut des canaux de données.
function decodeGatewayText(raw) {
	if (typeof raw === "string") {
		return raw;
	}
	if (raw instanceof ArrayBuffer) {
		return new TextDecoder().decode(new Uint8Array(raw));
	}
	if (ArrayBuffer.isView(raw)) {
		return new TextDecoder().decode(raw);
	}

	return null;
}

class RemoteplayInputService {
	constructor() {
		this.socket = null;
		this.sessionId = null;
		this.iceKey = null;
		this.boundSessionId = null;
		this.boundIceKey = null;
		this.boundViewerId = null;
		this.isConnecting = false;
		this.isManualDisconnect = false;
		this.reconnectAttempts = 0;
		this.reconnectTimer = null;

		// Phase B1: when set, controller messages ride the gateway's input
		// DataChannel (the retro/streamex path) instead of the input WS.
		this.gatewayInputSender = null;

		// LE MENU DE LA PARTIE OUVERT, LA CONSOLE N'ENTEND PLUS RIEN (#408) —
		// voir setSuspended. Toutes les entrées (manette, clavier, pad
		// tactile) passent par _sendController : c'est le seul endroit à
		// fermer.
		this.suspended = false;
		onGameInputSuspension((suspended) => this.setSuspended(suspended));

		this.rumbleListeners = new Set();
		this.stateListeners = new Set();
		this.healthUpdateListeners = new Set();
		this.devicesListeners = new Set();
		// L'ABONNEMENT À L'ÉTAT DES CONSOLES SURVIT À LA CONNEXION (2026-10-25).
		// Le serveur le range sur la connexion : une reconnexion (un proxy qui
		// coupe un WebSocket muet, un serveur redémarré) le perdait, et la
		// carte restait sur « Veille » jusqu'à ce qu'on revienne sur la page.
		// openSocket le renvoie à chaque connexion tant que ce drapeau est levé.
		this.devicesWanted = false;

		this.pendingPings = new Map(); // seq -> { resolve, timeout }
		this.pendingControllerState = null;

		// ICE restart callbacks kept for API compatibility — the backend
		// never pushed these events even over SignalR (they resolve via the
		// same fire-and-forget/timeout path as requestKeyframe).
		// chantier-4 — the gateway viewer id this page announced. It is what
		// the child's queue matches on, and it is NOT iceKey: iceKey collapses
		// to the sessionId for an owner, so two of the owner's own tabs would
		// be indistinguishable — and telling them apart is the whole model.
		this.viewerId = null;
	}

	// ── Transport ────────────────────────────────────────────────────────

	socketURL() {
		const params = new URLSearchParams();
		if (this.sessionId) params.set("sessionId", this.sessionId);
		if (this.iceKey) params.set("iceKey", this.iceKey);
		if (this.viewerId) params.set("viewerId", this.viewerId);
		// /api/v1/remoteplay/* now requires a valid JWT (kernel's
		// remoteplayRouter, previously the one API surface with no auth
		// check at all) — a browser can't attach an Authorization header
		// to a WS handshake, same reason retro-gaming's own stream WS
		// carries it on ?token= (use-retrogaming-streaming-connection.js).
		// LE JETON DE L'HÔTE VISÉ, pas le nôtre (chantier 9, C9-P4).
		//
		// Ce socket porte la manette. Présenter notre jeton à un serveur
		// fédéré le fait refuser la poignée de main — et le symptôme n'est
		// pas « refusé », c'est une partie sans manette du tout.
		const token = streamAuthToken() || getStoredToken();
		if (token) params.set("token", token);
		const query = params.toString();
		// Under /api/v1/remoteplay like the rest of a game's stream: that
		// domain belongs to the kernel alone, so exactly one proxy entry can
		// claim the upgrade — which is what the separate /api-ws prefix had
		// been invented for (2026-08-25, ERR_CONNECTION_RESET, no controller).
		return wsUrl(`/api/v1/remoteplay/input/ws${query ? `?${query}` : ""}`);
	}

	/**
	 * Connect (or rebind) the socket. sessionId is the Takion session scope;
	 * iceKey scopes ICE pushes (omit for the owner — defaults to sessionId
	 * server-side).
	 */
	connect(sessionId, iceKey) {
		this.sessionId = sessionId || this.sessionId;
		this.iceKey = iceKey || this.iceKey;
		if (!this.sessionId && !this.iceKey) {
			// Device-status-only connections (no session at all).
		}

		if (
			this.socket &&
			this.socket.readyState === WebSocket.OPEN &&
			!this.scopeChanged()
		) {
			return Promise.resolve();
		}
		this.isManualDisconnect = false;

		// Rebinding with different params reconnects the socket — the server
		// binding lives on the connection.
		if (this.socket) {
			try {
				this.socket.close();
			} catch {
				// ignore
			}
			this.socket = null;
		}

		return this.openSocket();
	}

	openSocket() {
		this.isConnecting = true;
		this.boundSessionId = this.sessionId;
		this.boundIceKey = this.iceKey;
		this.boundViewerId = this.viewerId;
		this.notifyStateChange({ isConnected: false, isConnecting: true });

		return new Promise((resolve, reject) => {
			const socket = new WebSocket(this.socketURL());
			this.socket = socket;

			socket.onopen = () => {
				this.isConnecting = false;
				this.reconnectAttempts = 0;
				debugTrace("connected", this.socketURL());
				// Jamais sur le socket d'une PARTIE : il mène au conteneur de
				// session, qui n'a pas la liste des consoles — la demande l'y
				// abattait (signalé sur la 0.4.3, 2026-10-25).
				if (this.devicesWanted && !this.sessionId) {
					this.send({ type: "subscribeDevices" });
				}
				this.notifyStateChange({ isConnected: true, isConnecting: false });
				resolve();
			};

			socket.onmessage = (event) => {
				let message;
				try {
					message = JSON.parse(event.data);
				} catch {
					return;
				}
				this.handleServerMessage(message);
			};

			socket.onclose = () => {
				if (this.socket === socket) this.socket = null;
				this.failPendingRequests();
				this.isConnecting = false;
				this.notifyStateChange({ isConnected: false, isConnecting: false });
				if (this.isManualDisconnect) return;
				this.scheduleReconnect();
			};

			socket.onerror = () => {
				reject(new Error("input websocket error"));
			};
		});
	}

	scopeChanged() {
		// The server binds sessionID/iceKey at connect time, so a different
		// scope (a viewer joining a session, say) needs a fresh socket.
		return (
			this.sessionId !== this.boundSessionId ||
			this.iceKey !== this.boundIceKey ||
			this.viewerId !== this.boundViewerId
		);
	}

	scheduleReconnect() {
		const delay = Math.min(
			RECONNECT_MAX_DELAY_MS,
			RECONNECT_MIN_DELAY_MS * 2 ** this.reconnectAttempts,
		);
		this.reconnectAttempts++;
		debugTrace("reconnecting in", delay, "ms");
		this.reconnectTimer = setTimeout(() => {
			this.openSocket().catch(() => { });
		}, delay);
	}

	async disconnect() {
		this.isManualDisconnect = true;
		clearTimeout(this.reconnectTimer);
		if (this.socket) {
			try {
				this.socket.close();
			} catch {
				// ignore
			}
		}
		this.socket = null;
		this.sessionId = null;
		this.iceKey = null;
		this.boundSessionId = null;
		this.boundIceKey = null;
		this.boundViewerId = null;
		this.failPendingRequests();
		this.notifyStateChange({ isConnected: false, isConnecting: false });
	}

	failPendingRequests() {
		for (const { resolve, timeout } of this.pendingPings.values()) {
			clearTimeout(timeout);
			resolve(null);
		}
		this.pendingPings.clear();
		if (this.pendingControllerState) {
			clearTimeout(this.pendingControllerState.timeout);
			this.pendingControllerState.reject(new Error("input websocket closed"));
			this.pendingControllerState = null;
		}
	}

	isConnected() {
		return (
			!!this.socket &&
			this.socket.readyState === WebSocket.OPEN &&
			!!this.sessionId
		);
	}

	// Can controller input actually go out right now? Mirrors what
	// _sendController really does: the gateway DataChannel first, the raw
	// socket only as a fallback. isConnected() above answers a DIFFERENT
	// question (is the raw socket up) and must not be used as an input gate
	// — the stick loop did exactly that and silently dropped every stick
	// update while the DataChannel was perfectly alive, which is why buttons
	// worked and analog sticks did not (reported live 2026-08-24).
	canSendInput() {
		return !!this.gatewayInputSender || (this.isSocketOpen() && !!this.sessionId);
	}

	isSocketOpen() {
		return !!this.socket && this.socket.readyState === WebSocket.OPEN;
	}

	send(message) {
		if (!this.isSocketOpen()) return false;
		this.socket.send(JSON.stringify(message));
		return true;
	}

	// ── Server -> client events ──────────────────────────────────────────

	handleServerMessage(message) {
		switch (message.type) {
			case "pong": {
				const pending = this.pendingPings.get(message.echo);
				if (pending) {
					clearTimeout(pending.timeout);
					this.pendingPings.delete(message.echo);
					pending.resolve(message);
				}
				break;
			}
			case "controllerState": {
				if (this.pendingControllerState) {
					clearTimeout(this.pendingControllerState.timeout);
					const resolve = this.pendingControllerState.resolve;
					this.pendingControllerState = null;
					resolve(message.state);
				}
				break;
			}
			case "rumble":
				this.notifyRumble(this.normalizeRumblePayload(message));
				break;
			case "health":
				this.healthUpdateListeners.forEach((listener) =>
					listener(message.health),
				);
				break;
			case "devices":
				this.devicesListeners.forEach((listener) => listener(message.devices));
				break;
			default:
				debugTrace("unhandled server message", message.type);
		}
	}

	// ── Controller surface (ex-controller.service.js) ────────────────────

	// Phase B1: the owner page registers the gateway DataChannel as the
	// controller transport once it opens — send* below then bypass the WS.
	setGatewayInputSender(sender) {
		this.gatewayInputSender = sender;
	}

	clearGatewayInputSender() {
		this.gatewayInputSender = null;
	}

	// B2: true once the gateway DataChannel carries the input — the hub
	// connection is then not needed (and binding it would re-create the
	// old phantom-holder problem).
	hasGatewaySender() {
		return Boolean(this.gatewayInputSender);
	}

	// B2 downlink: messages pushed from the child over the gateway input
	// DataChannel (rumble to the holder, etc.). Same dispatch shapes as
	// the hub _handleMessage path so listeners stay single-sourced.
	dispatchGatewayMessage(raw) {
		// DÉCODER AVANT DE PARSER. Le canal de données livre du BINAIRE —
		// le gateway écrit `DataChannel.Send([]byte)`, donc la page reçoit
		// un ArrayBuffer, jamais une chaîne.
		//
		// `JSON.parse` d'un ArrayBuffer ne lit pas son contenu : il le passe
		// par `String()`, obtient « [object ArrayBuffer] », et lève. Le
		// `catch` renvoyait alors sans rien dire, et TOUTES les vibrations
		// de la PS5 disparaissaient ici — le serveur journalisait pourtant
		// « envoyé à gino », et la sonde de la page disait « reçu par la
		// page — ArrayBuffer ». Les deux étaient vrais : le message arrivait
		// bien, et se faisait jeter à la ligne suivante (2026-09-09).
		//
		// `decodeGatewayText` existait déjà, écrite pour ce cas précis, et
		// n'était appelée nulle part — une fonction posée puis oubliée au
		// moment du câblage, le même motif que les autres pertes de la
		// refonte.
		const text = decodeGatewayText(raw);
		if (text === null) {
			console.warn("[remoteplay-input] message du gateway illisible:", raw);

			return;
		}

		let message;
		try {
			message = JSON.parse(text);
		} catch (error) {
			// Un JSON invalide n'est pas une forme qu'on ne sait pas lire :
			// c'est une anomalie, et la taire est ce qui a coûté ce bug.
			console.warn("[remoteplay-input] JSON du gateway invalide:", text.slice(0, 200));

			return;
		}
		if (message?.type === "rumble") {
			this.notifyRumble(this.normalizeRumblePayload(message.payload));
		}
	}

	// SE TAIRE EN RELÂCHANT TOUT. Les deux boutons tenus cinq secondes pour
	// ouvrir le menu — et tout ce qui était enfoncé à ce moment-là — sont
	// relâchés AVANT le silence : sinon la console les garderait enfoncés
	// jusqu'à la fermeture du menu. Relâcher un bouton qui ne l'était pas ne
	// fait rien.
	setSuspended(suspended) {
		if (suspended === this.suspended) {
			return;
		}
		if (suspended) {
			for (const button of RELEASABLE_BUTTONS) {
				this._sendController({ type: "button", button, action: "release", delayMs: 0 });
			}
			this._sendController({ type: "sticks", lx: 0, ly: 0, rx: 0, ry: 0 });
			this._sendController({ type: "triggers", l2: 0, r2: 0 });
		}
		this.suspended = suspended;
	}

	_sendController(message) {
		if (this.suspended) {
			return;
		}
		if (this.gatewayInputSender) {
			this.gatewayInputSender.send(message);
			return;
		}
		if (!this.isSocketOpen() || !this.sessionId) return;
		this.send(message);
	}

	async sendButton(buttonName, action = "tap", delayMs = 0) {
		const actualDelay = action === "tap" ? delayMs || 100 : delayMs || 0;
		this._sendController({
			type: "button",
			button: buttonName,
			action,
			delayMs: actualDelay,
		});
	}

	async sendStick(stickType, x, y) {
		const type = stickType === "LEFT" ? "leftStick" : "rightStick";
		this._sendController({ type, x, y });
	}

	async sendSticks(leftX, leftY, rightX, rightY) {
		this._sendController({ type: "sticks", lx: leftX, ly: leftY, rx: rightX, ry: rightY });
	}

	async sendTriggers(l2, r2) {
		const clampedL2 = Math.max(0, Math.min(1, l2));
		const clampedR2 = Math.max(0, Math.min(1, r2));
		this._sendController({ type: "triggers", l2: clampedL2, r2: clampedR2 });
	}

	// One finger on the touchpad: which one (0 or 1), down or lifted, and
	// where — from 0 to 1 from the top-left corner. The host scales it to the
	// console's own pad (a DualShock 4 pad is shorter than a DualSense one),
	// so the page never needs to know which console it is talking to.
	async sendTouch(finger, down, x, y) {
		this._sendController({ type: "touch", finger, down, x, y });
	}

	/**
	 * Round-trip probe (profil/GamepadParameters.jsx "Connexion serveur") —
	 * the server echoes the sequence number with its own timestamp.
	 */
	async ping(seq) {
		if (!this.isSocketOpen()) throw new Error("input websocket unavailable");
		const clientSentAt = Date.now();
		return new Promise((resolve) => {
			const timeout = setTimeout(() => {
				this.pendingPings.delete(seq);
				resolve(null);
			}, REQUEST_TIMEOUT_MS);
			this.pendingPings.set(seq, { resolve, timeout });
			if (!this.send({ type: "ping", echo: seq })) {
				clearTimeout(timeout);
				this.pendingPings.delete(seq);
				resolve(null);
			}
		}).then((result) =>
			result ? { ...result, clientSentAt, clientReceivedAt: Date.now() } : null,
		);
	}

	/**
	 * Reads back the server's own interpreted ControllerState for this
	 * session — the other half of GamepadParameters.jsx's real-controller
	 * diagnostic.
	 */
	getControllerState() {
		if (!this.isSocketOpen() || !this.sessionId) {
			return Promise.reject(new Error("input websocket unavailable"));
		}
		return new Promise((resolve, reject) => {
			this.pendingControllerState = {
				resolve,
				reject,
				timeout: setTimeout(() => {
					this.pendingControllerState = null;
					reject(new Error("getControllerState timeout"));
				}, REQUEST_TIMEOUT_MS),
			};
			if (!this.send({ type: "getControllerState" })) {
				clearTimeout(this.pendingControllerState.timeout);
				this.pendingControllerState = null;
				reject(new Error("input websocket send failed"));
			}
		});
	}

	onRumble(listener) {
		this.rumbleListeners.add(listener);
		return () => this.rumbleListeners.delete(listener);
	}

	onStateChange(listener) {
		this.stateListeners.add(listener);
		return () => this.stateListeners.delete(listener);
	}

	notifyStateChange(state) {
		this.stateListeners.forEach((listener) => listener(state));
	}

	normalizeRumblePayload(payload) {
		if (!payload || typeof payload !== "object") return null;

		const ensureNumber = (value, fallback) =>
			typeof value === "number" && Number.isFinite(value) ? value : fallback;

		const clampToByte = (value) => {
			if (!Number.isFinite(value)) return 0;
			if (value <= 0) return 0;
			if (value >= 255) return 255;
			return Math.round(value);
		};

		const rawLeft = clampToByte(
			ensureNumber(payload.rawLeft ?? payload.left, 0),
		);
		const rawRight = clampToByte(
			ensureNumber(payload.rawRight ?? payload.right, 0),
		);
		const adjustedLeft = clampToByte(
			ensureNumber(payload.left ?? payload.rawLeft, rawLeft),
		);
		const adjustedRight = clampToByte(
			ensureNumber(payload.right ?? payload.rawRight, rawRight),
		);

		return {
			unknown: clampToByte(ensureNumber(payload.unknown, 0)),
			rawLeft,
			rawRight,
			left: adjustedLeft,
			right: adjustedRight,
			multiplier: ensureNumber(payload.multiplier, 1),
			ps5RumbleIntensity: ensureNumber(payload.ps5RumbleIntensity, 0),
			ps5TriggerIntensity: ensureNumber(payload.ps5TriggerIntensity, 0),
			timestamp:
				typeof payload.timestamp === "string" ? payload.timestamp : null,
		};
	}

	notifyRumble(event) {
		this.rumbleListeners.forEach((listener) => {
			try {
				listener(event);
			} catch (error) {
				console.warn("Rumble event handling failed:", error);
			}
		});
	}

	// ── Streaming surface (ex-streaming-hub.service.js) ──────────────────

	async joinSession(sessionId, iceKey, viewerId = null) {
		if (!sessionId) return;
		// The socket binds at connect time — rebind when the scope changes
		// (a viewer passes its own viewerId as iceKey).
		if (
			this.sessionId !== sessionId ||
			this.iceKey !== (iceKey || sessionId) ||
			(viewerId && this.viewerId !== viewerId)
		) {
			this.sessionId = sessionId;
			this.iceKey = iceKey || sessionId;
			if (viewerId) this.viewerId = viewerId;
			try {
				await this.connect(sessionId, this.iceKey);
			} catch {
				// best-effort, ignore
			}
		}
	}

	async requestKeyframe() {
		if (!this.isSocketOpen() || !this.sessionId) return false;
		this.send({ type: "requestKeyframe" });
		// Fire-and-forget, same observable behavior as before (the server
		// never acknowledged these even over SignalR).
		return true;
	}

	async forceResetReorderQueue() {
		if (!this.isSocketOpen() || !this.sessionId) return false;
		this.send({ type: "forceResetReorderQueue" });
		return true;
	}

	async reportNetworkLoss(ratio) {
		if (typeof ratio !== "number" || !Number.isFinite(ratio)) return;
		this.send({ type: "reportNetworkStats", ratio });
	}

	onHealthUpdate(listener) {
		this.healthUpdateListeners.add(listener);
		return () => this.healthUpdateListeners.delete(listener);
	}

	// ── Device-status surface (ex-device-status-hub.service.js) ──────────

	onDevices(listener) {
		this.devicesListeners.add(listener);
		return () => this.devicesListeners.delete(listener);
	}

	subscribeDevices() {
		this.devicesWanted = true;
		if (!this.isSocketOpen()) {
			// L'ouverture s'abonne d'elle-même (openSocket).
			return this.connect(null, null);
		}
		// Le socket d'une partie ne porte pas la liste : l'abonnement attend
		// la prochaine connexion au service (openSocket).
		if (!this.sessionId) {
			this.send({ type: "subscribeDevices" });
		}
		return Promise.resolve();
	}
}

// Singleton instance — one socket per page role, shared by the hooks.
export const remoteplayInputService = new RemoteplayInputService();
