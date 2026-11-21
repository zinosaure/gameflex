import { useCallback, useRef } from "react";
import { MOUSE_SENSITIVITY, getTimestamp, clamp } from "./constants";
import { debugTrace } from "@/gameflex/shared/utils/log-store";

export const useMouseRightStick = ({ videoRef, onPointerLockChange, onMouseMove }) => {
    const cleanupRef = useRef(null);

    const tearDown = useCallback(() => {
        if (cleanupRef.current) {
            cleanupRef.current();
            cleanupRef.current = null;
        }
    }, []);

    const setup = useCallback(() => {
        tearDown();

        const videoElement = videoRef.current;
        if (!videoElement) {
            console.warn("⚠️ Video element not found, cannot enable mouse control");
            onPointerLockChange(false);
            return;
        }

        const handleMouseDown = (event) => {
            if (event.button === 0) {
                event.preventDefault();
                videoElement.focus?.();
                if (document.pointerLockElement !== videoElement) {
                    videoElement.requestPointerLock?.();
                }
            }
        };

        const handleContextMenu = (event) => {
            if (document.pointerLockElement === videoElement) {
                event.preventDefault();
            }
        };

        const handlePointerLockChange = () => {
            const isLocked = document.pointerLockElement === videoElement;
            onPointerLockChange(isLocked);
        };

        const handlePointerLockError = (event) => {
            console.warn("⚠️ Pointer Lock failed to activate:", event);
            onPointerLockChange(false);
        };

        const handleMouseMove = (event) => {
            if (document.pointerLockElement !== videoElement) {
                return;
            }

            const now = getTimestamp();
            const nextX = clamp(event.movementX * MOUSE_SENSITIVITY);
            const nextY = clamp(event.movementY * MOUSE_SENSITIVITY);
            onMouseMove(nextX, nextY, now);
        };

        const handleWindowBlur = () => {
            if (document.pointerLockElement === videoElement) {
                document.exitPointerLock?.();
            } else {
                onPointerLockChange(false);
            }
        };

        videoElement.addEventListener("mousedown", handleMouseDown);
        videoElement.addEventListener("contextmenu", handleContextMenu);
        document.addEventListener("pointerlockchange", handlePointerLockChange);
        document.addEventListener("pointerlockerror", handlePointerLockError);
        document.addEventListener("mousemove", handleMouseMove);
        window.addEventListener("blur", handleWindowBlur);

        cleanupRef.current = () => {
            videoElement.removeEventListener("mousedown", handleMouseDown);
            videoElement.removeEventListener("contextmenu", handleContextMenu);
            document.removeEventListener("pointerlockchange", handlePointerLockChange);
            document.removeEventListener("pointerlockerror", handlePointerLockError);
            document.removeEventListener("mousemove", handleMouseMove);
            window.removeEventListener("blur", handleWindowBlur);

            if (document.pointerLockElement === videoElement) {
                document.exitPointerLock?.();
            }

            onPointerLockChange(false);
        };

        onPointerLockChange(false);
        debugTrace("✅ Mouse control enabled (right stick emulation)");
    }, [onPointerLockChange, onMouseMove, tearDown, videoRef]);

    return {
        setup,
        tearDown,
    };
};
