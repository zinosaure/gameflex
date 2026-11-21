/**
 * Device detection and mobile utility functions
 */

/**
 * Detect if the current device is a mobile device.
 * Includes real mobile devices and Chrome DevTools device emulator.
 */
export function isMobileDevice() {
    if (typeof window === "undefined") return false;

    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;

    // Check viewport size first — Chrome DevTools emulator changes the viewport
    if (viewportWidth <= 768) {
        // Check aspect ratio typical of mobile devices (portrait or landscape)
        const isPortraitMobile =
            viewportHeight > viewportWidth && viewportHeight / viewportWidth > 1.3;
        const isLandscapeMobile =
            viewportWidth > viewportHeight && viewportWidth / viewportHeight > 1.3;
        const isSquareMobile = viewportWidth <= 768 && viewportHeight <= 1024;

        if (isPortraitMobile || isLandscapeMobile || isSquareMobile) {
            return true;
        }
    }

    // Check user agent for real mobile devices
    const userAgent = navigator.userAgent || navigator.vendor || window.opera;
    const mobileRegex = /android|webos|iphone|ipad|ipod|blackberry|iemobile|opera mini/i;
    if (mobileRegex.test(userAgent)) {
        return true;
    }

    // Touch support + small screen is a reliable mobile signal
    // Note: touch support alone is not enough — desktops can have touch screens too
    const hasTouchSupport = "ontouchstart" in window || navigator.maxTouchPoints > 0;
    if (hasTouchSupport && viewportWidth <= 768) {
        return true;
    }

    return false;
}

/**
 * Detect if the current device is a tablet.
 * Includes real tablets and Chrome DevTools tablet emulator.
 */
export function isTabletDevice() {
    if (typeof window === "undefined") return false;

    const viewportWidth = window.innerWidth;

    // Tablets are typically 768px–1024px wide
    const isTabletSize = viewportWidth >= 768 && viewportWidth <= 1024;
    const hasTouchSupport = "ontouchstart" in window || navigator.maxTouchPoints > 0;

    if (isTabletSize && hasTouchSupport) {
        return true;
    }

    // Check user agent for real tablet devices
    const userAgent = navigator.userAgent || navigator.vendor || window.opera;
    const tabletRegex = /ipad|android(?!.*mobile)|tablet/i;
    if (tabletRegex.test(userAgent)) {
        return true;
    }

    return false;
}

/**
 * Enter fullscreen mode.
 */
export async function requestFullscreen(element) {
    if (element.requestFullscreen) {
        await element.requestFullscreen();
    } else if (element.webkitRequestFullscreen) {
        await element.webkitRequestFullscreen();
    } else if (element.mozRequestFullScreen) {
        await element.mozRequestFullScreen();
    } else if (element.msRequestFullscreen) {
        await element.msRequestFullscreen();
    }
}

/**
 * Exit fullscreen mode.
 */
export async function exitFullscreen() {
    if (document.exitFullscreen) {
        await document.exitFullscreen();
    } else if (document.webkitExitFullscreen) {
        await document.webkitExitFullscreen();
    } else if (document.mozCancelFullScreen) {
        await document.mozCancelFullScreen();
    } else if (document.msExitFullscreen) {
        await document.msExitFullscreen();
    }
}

/**
 * Detect if the document is currently in fullscreen.
 */
export function isFullscreen() {
    if (typeof document === "undefined") return false;

    return !!(
        document.fullscreenElement ||
        document.webkitFullscreenElement ||
        document.mozFullScreenElement ||
        document.msFullscreenElement
    );
}

/**
 * Detect if the device is in portrait orientation.
 */
export function isPortrait() {
    if (typeof window === "undefined") return false;
    return window.innerHeight > window.innerWidth;
}
