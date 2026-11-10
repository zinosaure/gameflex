import { useEffect } from 'react';

let chainOrigin = null;

// Restore by stable identity when a refresh replaced the original button. Defer
// restoration so a transition from the shared sheet to launch keeps its focus.
export function useDialogFocus(root) {
    useEffect(() => {
        const opener = document.activeElement;
        const origin = chainOrigin && (opener === document.body || opener?.closest('[aria-modal="true"]')) ? chainOrigin : { opener, identity: opener?.getAttribute('data-focus-key') };
        chainOrigin = origin;
        root.current?.querySelector('button:not(:disabled),select,a[href]')?.focus({ preventScroll: true });
        return () => queueMicrotask(() => {
            if (document.querySelector('[aria-modal="true"]')) return;
            const { opener: original, identity } = origin;
            const replacement = identity && [...document.querySelectorAll('[data-focus-key]')].find(el => el.getAttribute('data-focus-key') === identity);
            const target = original?.isConnected ? original : replacement || document.querySelector('main h1');
            if (target?.tagName === 'H1') target.tabIndex = -1;
            target?.focus({ preventScroll: true });
            if (chainOrigin === origin) chainOrigin = null;
        });
    }, [root]);
}
