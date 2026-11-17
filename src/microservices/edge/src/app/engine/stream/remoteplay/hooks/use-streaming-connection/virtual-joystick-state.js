// Global virtual joystick state (not a React Hook, ensures all components share the same state)

// Global virtual joystick state
export const virtualStickState = {
    leftStick: { x: 0, y: 0 },
    rightStick: { x: 0, y: 0 },
};

// Whether the virtual joystick is active
export const virtualJoystickActiveState = {
    left: false,
    right: false,
};

// Set virtual joystick value
export function setVirtualStick(stick, x, y) {
    const target = stick === "left" ? virtualStickState.leftStick : virtualStickState.rightStick;
    target.x = Math.max(-1, Math.min(1, x));
    target.y = Math.max(-1, Math.min(1, y));
}

// Set virtual joystick active state
export function setVirtualStickActive(stick, active) {
    if (stick === "left") {
        virtualJoystickActiveState.left = active;
    } else {
        virtualJoystickActiveState.right = active;
    }
}
