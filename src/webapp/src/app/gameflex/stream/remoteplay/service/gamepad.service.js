import { debugTrace } from "@/gameflex/shared/utils/log-store";
import { isMenuHome } from "@/gameflex/libs/gameInput.js";

/**
 * Gamepad service
 * Uses the Web Gamepad API to detect and connect gamepad devices
 */

// Gamepad connection state

// Gamepad info

// Gamepad button state

// Gamepad input event

// Gamepad button mapping (standard Xbox/PS controller)
export let GamepadButton = /*#__PURE__*/ (function (GamepadButton) {
	// Standard buttons
	GamepadButton[(GamepadButton["A"] = 0)] = "A"; // Bottom button (Xbox A, PS X)
	GamepadButton[(GamepadButton["B"] = 1)] = "B"; // Right button (Xbox B, PS Circle)
	GamepadButton[(GamepadButton["X"] = 2)] = "X"; // Left button (Xbox X, PS Square)
	GamepadButton[(GamepadButton["Y"] = 3)] = "Y"; // Top button (Xbox Y, PS Triangle)

	// Shoulder buttons
	GamepadButton[(GamepadButton["LeftShoulder"] = 4)] = "LeftShoulder"; // LB/L1
	GamepadButton[(GamepadButton["RightShoulder"] = 5)] = "RightShoulder"; // RB/R1
	GamepadButton[(GamepadButton["LeftTrigger"] = 6)] = "LeftTrigger"; // LT/L2 (usually as axis)
	GamepadButton[(GamepadButton["RightTrigger"] = 7)] = "RightTrigger"; // RT/R2 (usually as axis)

	// Function buttons
	GamepadButton[(GamepadButton["Back"] = 8)] = "Back"; // Select/Share
	GamepadButton[(GamepadButton["Start"] = 9)] = "Start"; // Start/Options
	GamepadButton[(GamepadButton["LeftStick"] = 10)] = "LeftStick"; // Left stick press
	GamepadButton[(GamepadButton["RightStick"] = 11)] = "RightStick"; // Right stick press

	// D-Pad (usually as buttons 12-15)
	GamepadButton[(GamepadButton["DPadUp"] = 12)] = "DPadUp";
	GamepadButton[(GamepadButton["DPadDown"] = 13)] = "DPadDown";
	GamepadButton[(GamepadButton["DPadLeft"] = 14)] = "DPadLeft";
	GamepadButton[(GamepadButton["DPadRight"] = 15)] = "DPadRight";

	// The guide button (PS / Xbox / Home). The enum stopped at 15, so the one
	// button a PlayStation player reaches for by reflex reached nothing — the
	// on-screen pad has had it all along (presets.js "PS"), the physical one
	// never did (reported live 2026-08-25). Not every browser/driver exposes
	// it: some capture it before the page sees it, which is a reason to test
	// on the real pad, not a reason to leave it unmapped.
	GamepadButton[(GamepadButton["Guide"] = 16)] = "Guide";

	// The touchpad CLICK, index 17 on a DualShock 4 / DualSense under the
	// standard mapping. Same asymmetry as the guide button: presets.js draws a
	// TOUCHPAD button, and pressing the real one reached nothing. This is the
	// click only — the touch SURFACE (coordinates) is a separate piece of
	// protocol, still open in docs/TODO.md.
	GamepadButton[(GamepadButton["Touchpad"] = 17)] = "Touchpad";
	return GamepadButton;
})({});

// Gamepad axis index
export let GamepadAxis = /*#__PURE__*/ (function (GamepadAxis) {
	GamepadAxis[(GamepadAxis["LeftStickX"] = 0)] = "LeftStickX";
	GamepadAxis[(GamepadAxis["LeftStickY"] = 1)] = "LeftStickY";
	GamepadAxis[(GamepadAxis["RightStickX"] = 2)] = "RightStickX";
	GamepadAxis[(GamepadAxis["RightStickY"] = 3)] = "RightStickY";
	return GamepadAxis;
})({});

// PlayStation button name mapping
export const PS5_BUTTON_MAP = {
	[GamepadButton.A]: "cross", // A/X button
	[GamepadButton.Y]: "triangle", // Y/Triangle button
	[GamepadButton.X]: "square", // B/Square button
	[GamepadButton.B]: "circle", // A/Circle button
	[GamepadButton.LeftShoulder]: "l1",
	[GamepadButton.RightShoulder]: "r1",
	[GamepadButton.LeftTrigger]: "l2",
	[GamepadButton.RightTrigger]: "r2",
	[GamepadButton.Back]: "share",
	[GamepadButton.Start]: "options",
	[GamepadButton.LeftStick]: "l3",
	[GamepadButton.RightStick]: "r3",
	[GamepadButton.DPadUp]: "up",
	[GamepadButton.DPadDown]: "down",
	[GamepadButton.DPadLeft]: "left",
	[GamepadButton.DPadRight]: "right",
	// Lower-case like its neighbours: the child upper-cases every button name
	// before looking it up (input_hub.go), and "PS" is a name Takion knows.
	[GamepadButton.Guide]: "ps",
	[GamepadButton.Touchpad]: "touchpad",
};

class GamepadService {
	gamepads = new Map();
	connectedGamepads = [];
	isPolling = false;
	pollingInterval = null;
	stateListeners = new Set();
	inputListeners = new Set();
	previousButtonStates = new Map();
	previousButtonValues = new Map();
	previousAxisStates = new Map();

	constructor() {
		this.setupEventListeners();
	}

	/**
	 * Set up event listeners
	 */
	setupEventListeners() {
		// Listen for gamepad connected
		window.addEventListener("gamepadconnected", (e) => {
			debugTrace(
				"🎮 Gamepad connected:",
				e.gamepad.id,
				"index:",
				e.gamepad.index,
			);
			this.handleGamepadConnected(e.gamepad);
		});

		// Listen for gamepad disconnected
		window.addEventListener("gamepaddisconnected", (e) => {
			debugTrace(
				"🎮 Gamepad disconnected:",
				e.gamepad.id,
				"index:",
				e.gamepad.index,
			);
			this.handleGamepadDisconnected(e.gamepad.index);
		});

		// Check already connected gamepads on init
		this.scanGamepads();
	}

	/**
	 * Scan connected gamepads
	 */
	scanGamepads() {
		const gamepads = navigator.getGamepads();
		for (let i = 0; i < gamepads.length; i++) {
			const gamepad = gamepads[i];
			if (gamepad) {
				this.handleGamepadConnected(gamepad);
			}
		}
	}

	/**
	 * Handle gamepad connected
	 */
	handleGamepadConnected(gamepad) {
		this.gamepads.set(gamepad.index, gamepad);

		const info = {
			index: gamepad.index,
			id: gamepad.id,
			mapping: gamepad.mapping,
			axes: gamepad.axes.length,
			buttons: gamepad.buttons.length,
			timestamp: gamepad.timestamp,
		};

		// Update connected gamepads list
		const existingIndex = this.connectedGamepads.findIndex(
			(g) => g.index === gamepad.index,
		);
		if (existingIndex >= 0) {
			this.connectedGamepads[existingIndex] = info;
		} else {
			this.connectedGamepads.push(info);
		}

		// Initialize button and axis states
		this.previousButtonStates.set(
			gamepad.index,
			new Array(gamepad.buttons.length).fill(false),
		);
		this.previousButtonValues.set(
			gamepad.index,
			new Array(gamepad.buttons.length).fill(0),
		);
		this.previousAxisStates.set(
			gamepad.index,
			new Array(gamepad.axes.length).fill(0),
		);

		this.notifyStateChange();
		this.startPolling();
	}

	/**
	 * Handle gamepad disconnected
	 */
	handleGamepadDisconnected(index) {
		this.disconnectGamepad(index);
	}

	/**
	 * Manually disconnect gamepad (remove from internal state)
	 */
	disconnectGamepad(index) {
		debugTrace("🎮 Manually disconnecting gamepad:", index);
		this.gamepads.delete(index);
		this.connectedGamepads = this.connectedGamepads.filter(
			(g) => g.index !== index,
		);
		this.previousButtonStates.delete(index);
		this.previousButtonValues.delete(index);
		this.previousAxisStates.delete(index);

		this.notifyStateChange();

		// If no connected gamepads, stop polling
		if (this.connectedGamepads.length === 0) {
			this.stopPolling();
		}
	}

	/**
	 * Disconnect by gamepad ID
	 */
	disconnectGamepadById(gamepadId) {
		const gamepad = this.connectedGamepads.find((g) => g.id === gamepadId);
		if (gamepad) {
			this.disconnectGamepad(gamepad.index);
			return true;
		}
		return false;
	}

	/**
	 * Start polling gamepad input
	 */
	startPolling() {
		if (this.isPolling) {
			return;
		}

		this.isPolling = true;
		const poll = () => {
			if (!this.isPolling) {
				return;
			}

			// Update all gamepad states
			const gamepads = navigator.getGamepads();
			for (let i = 0; i < gamepads.length; i++) {
				const gamepad = gamepads[i];
				if (gamepad && this.gamepads.has(gamepad.index)) {
					this.updateGamepadState(gamepad);
				}
			}

			// Continue polling
			this.pollingInterval = requestAnimationFrame(poll);
		};

		this.pollingInterval = requestAnimationFrame(poll);
	}

	/**
	 * Stop polling
	 */
	stopPolling() {
		this.isPolling = false;
		if (this.pollingInterval !== null) {
			cancelAnimationFrame(this.pollingInterval);
			this.pollingInterval = null;
		}
	}

	/**
	 * Update gamepad state and trigger events
	 */
	updateGamepadState(gamepad) {
		const previousButtons = this.previousButtonStates.get(gamepad.index) || [];
		const previousButtonValues =
			this.previousButtonValues.get(gamepad.index) || [];
		const previousAxes = this.previousAxisStates.get(gamepad.index) || [];

		// Check button state changes
		for (let i = 0; i < gamepad.buttons.length; i++) {
			const button = gamepad.buttons[i];
			const previousPressed = previousButtons[i] || false;
			const previousValue = previousButtonValues[i] ?? 0;
			// L2 tenu, Home est le raccourci du menu (libs/gameInput.js) :
			// lu comme lâché, il n'appuie pas sur PS à chaque ouverture.
			const menuHome = isMenuHome(gamepad, i);
			const currentPressed = button.pressed && !menuHome;
			const currentValue = menuHome ? 0 : button.value;
			const valueChanged = Math.abs(currentValue - previousValue) > 0.00001;

			if (previousPressed !== currentPressed || valueChanged) {
				this.notifyInput({
					gamepadIndex: gamepad.index,
					buttonIndex: i,
					buttonState: {
						pressed: currentPressed,
						value: currentValue,
						touched: button.touched,
					},
				});
				previousButtons[i] = currentPressed;
				previousButtonValues[i] = currentValue;
			} else {
				previousButtonValues[i] = currentValue;
			}
		}

		// Check axis state changes (very low threshold to capture all stick inputs)
		for (let i = 0; i < gamepad.axes.length; i++) {
			const axisValue = gamepad.axes[i];
			const previousValue = previousAxes[i] || 0;
			const threshold = 0.001; // Very low threshold, almost any change will trigger

			// Trigger event if value changed (above threshold) or value itself is large (above dead zone)
			if (
				Math.abs(axisValue - previousValue) > threshold ||
				Math.abs(axisValue) > 0.001
			) {
				this.notifyInput({
					gamepadIndex: gamepad.index,
					axisIndex: i,
					axisValue: axisValue,
				});
				previousAxes[i] = axisValue;
			}
		}

		// Update state
		this.previousButtonStates.set(gamepad.index, previousButtons);
		this.previousButtonValues.set(gamepad.index, previousButtonValues);
		this.previousAxisStates.set(gamepad.index, previousAxes);
	}

	/**
	 * Get connection state
	 */
	getConnectionState() {
		return {
			isConnected: this.connectedGamepads.length > 0,
			connectedGamepads: [...this.connectedGamepads],
		};
	}

	/**
	 * Get all connected gamepads
	 */
	getConnectedGamepads() {
		return [...this.connectedGamepads];
	}

	/**
	 * Check if any gamepad is connected
	 */
	hasGamepad() {
		return this.connectedGamepads.length > 0;
	}

	/**
	 * Get gamepad instance
	 */
	getGamepad(index) {
		const gamepads = navigator.getGamepads();
		return gamepads[index] || null;
	}

	/**
	 * Add state change listener
	 */
	onStateChange(listener) {
		this.stateListeners.add(listener);
		return () => {
			this.stateListeners.delete(listener);
		};
	}

	/**
	 * Add input event listener
	 */
	onInput(listener) {
		this.inputListeners.add(listener);
		return () => {
			this.inputListeners.delete(listener);
		};
	}

	/**
	 * Notify state change
	 */
	notifyStateChange() {
		const state = this.getConnectionState();
		this.stateListeners.forEach((listener) => listener(state));
	}

	/**
	 * Notify input event
	 */
	notifyInput(event) {
		this.inputListeners.forEach((listener) => listener(event));
	}

	/**
	 * Clean up resources
	 */
	dispose() {
		this.stopPolling();
		this.stateListeners.clear();
		this.inputListeners.clear();
		this.gamepads.clear();
		this.connectedGamepads = [];
		this.previousButtonStates.clear();
		this.previousAxisStates.clear();
	}
}

// Singleton instance
export const gamepadService = new GamepadService();
