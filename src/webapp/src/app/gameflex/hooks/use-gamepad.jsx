import React, {
	createContext,
	useContext,
	useEffect,
	useState,
	useRef,
} from "react";
import { useTranslation } from "react-i18next";
import { gamepadService } from "@/gameflex/stream/remoteplay/service/gamepad.service";
import Skeleton from "../../skeleton/skeleton.js";
import { debugTrace } from "@/gameflex/shared/utils/log-store";

const GamepadContext = createContext(undefined);

// This provider is used outside the Skeleton router tree in some contexts,
// so `toast` is a small local shim over Skeleton.context — not the shadcn
// use-toast hook from the source app (never ported, superseded by
// Skeleton's own toast system, see CODE_CONVENTIONS.md).
export function GamepadProvider({ children }) {
	const skeletonContext = useContext(Skeleton.context);
	// Les messages de branchement étaient écrits en anglais, en dur.
	const { t } = useTranslation();
	// Stable ref, not a plain function recreated every render — the effect
	// below must only subscribe once on mount (see its now-empty deps
	// array). Depending on a freshly-recreated `toast` function was causing
	// an infinite effect teardown/setup loop: the effect's own
	// setConnectionState calls trigger a re-render, which recreates `toast`,
	// which re-triggers the effect ("Maximum update depth exceeded").
	const toastRef = useRef();
	toastRef.current = ({ title, description, duration }) => {
		skeletonContext.toast.open(
			description ? `${title} — ${description}` : title,
			{ duration: duration || 4000 },
		);
	};
	const [connectionState, setConnectionState] = useState(() =>
		gamepadService.getConnectionState(),
	);
	const [isEnabled, setIsEnabled] = useState(() => {
		// Read saved state from localStorage
		const saved = localStorage.getItem("gamepad_enabled");
		return saved !== "false"; // Default to true
	});
	const previousGamepadCountRef = useRef(0);
	const previousGamepadIdsRef = useRef(new Set());

	useEffect(() => {
		let isInitialized = false;

		// Get current state on init (don't show notifications)
		const initialState = gamepadService.getConnectionState();
		setConnectionState(initialState);
		previousGamepadCountRef.current = initialState.connectedGamepads.length;
		initialState.connectedGamepads.forEach((g) =>
			previousGamepadIdsRef.current.add(g.id),
		);

		// Delay marking as initialized to avoid showing notifications on startup
		const initTimer = setTimeout(() => {
			isInitialized = true;
		}, 500);

		// Subscribe to gamepad state changes
		const unsubscribe = gamepadService.onStateChange((state) => {
			// If not yet initialized, only update state, don't show notifications
			if (!isInitialized) {
				setConnectionState(state);
				previousGamepadCountRef.current = state.connectedGamepads.length;
				previousGamepadIdsRef.current = new Set(
					state.connectedGamepads.map((g) => g.id),
				);
				return;
			}

			const currentCount = state.connectedGamepads.length;
			const previousIds = previousGamepadIdsRef.current;
			const currentIds = new Set(state.connectedGamepads.map((g) => g.id));

			// Detect newly connected gamepads
			state.connectedGamepads.forEach((gamepad) => {
				if (!previousIds.has(gamepad.id)) {
					// New gamepad connected
					const gamepadName = gamepad.id || "Gamepad";
					// Simplify gamepad name display (remove technical details)
					const displayName =
						gamepadName
							.replace(/\(.*?\)/g, "") // Remove content inside parentheses
							.replace(/\s+/g, " ") // Merge multiple spaces
							.trim() || "Gamepad";

					toastRef.current({
						title: t("streaming.gamepad.connected", { name: displayName }),
						duration: 3000,
					});
					debugTrace(
						"🎮 Gamepad connected:",
						gamepad.id,
						"index:",
						gamepad.index,
					);
				}
			});

			// Detect disconnected gamepads
			previousIds.forEach((gamepadId) => {
				if (!currentIds.has(gamepadId)) {
					// Gamepad disconnected
					toastRef.current({
						title: t("streaming.gamepad.disconnected"),
						duration: 3000,
					});
					debugTrace("🎮 Gamepad disconnected:", gamepadId);
				}
			});

			// Update state
			setConnectionState(state);
			previousGamepadCountRef.current = currentCount;
			previousGamepadIdsRef.current = currentIds;
		});

		return () => {
			clearTimeout(initTimer);
			unsubscribe();
		};
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, []);

	const setEnabled = (enabled) => {
		setIsEnabled(enabled);
		localStorage.setItem("gamepad_enabled", enabled.toString());
	};

	const value = {
		isConnected: connectionState.isConnected,
		connectedGamepads: connectionState.connectedGamepads,
		hasGamepad: () => gamepadService.hasGamepad(),
		getGamepad: (index) => gamepadService.getGamepad(index),
		isEnabled,
		setEnabled,
		disconnectGamepad: (index) => gamepadService.disconnectGamepad(index),
		disconnectGamepadById: (gamepadId) =>
			gamepadService.disconnectGamepadById(gamepadId),
	};

	return (
		<GamepadContext.Provider value={value}>{children}</GamepadContext.Provider>
	);
}

export function useGamepad() {
	const context = useContext(GamepadContext);
	if (context === undefined) {
		throw new Error("useGamepad must be used within a GamepadProvider");
	}
	return context;
}

/**
 * Hook: Listen for gamepad input events
 */
export function useGamepadInput(onInput, enabled = true) {
	useEffect(() => {
		if (!enabled) {
			return;
		}

		const unsubscribe = gamepadService.onInput(onInput);

		return () => {
			unsubscribe();
		};
	}, [onInput, enabled]);
}
