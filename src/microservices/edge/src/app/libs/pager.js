import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { playSound } from "./sounds.js";

// LE DÉFILEMENT VERTICAL, UNE PAGE PAR ÉCRAN — les consoles d'un onglet, les
// fiches d'une bibliothèque (le porteur, 2026-11-12 : « au lieu de faire le
// défilement en horizontal on va le faire en vertical »). Écrit une fois, pour
// que les deux écrans aient exactement le même mouvement.
//
// Une seule position continue (`pos`) : chaque page glisse de toute la hauteur,
// son visuel (`.pvis`) ne parcourt qu'une partie du chemin (parallaxe), son
// texte (`.pinfo`) un peu plus et s'efface, la page qui part s'assombrit
// (`.pdim`). Un ressort amorti mène à la cible : une croix tenue fait filer les
// pages, qui s'arrêtent net. Au doigt, la page suit le doigt et s'aimante ;
// à la molette ou au pavé tactile, une page par geste. Pas de boucle : aux deux
// bouts, un petit rebond.
//
// Seules la page courante et ses deux voisines existent (`range`) : une console
// de dix mille jeux n'en construit que trois. Les transformations se posent
// directement sur le DOM à chaque image — passer par l'état de React à 60 images/s
// referait tout l'écran.

const clampTo = (value, low, high) => Math.max(low, Math.min(high, value));
const STIFFNESS = 120;
const BOUNCE = 2.6;
const WHEEL_QUIET_MS = 260;

export function useVerticalPager({ count, onTarget, onSettle, dragEnabled = true }) {
    // L'hôte peut n'apparaître qu'après un chargement (la bibliothèque) : une
    // référence-fonction, pour (re)brancher le glisser quand il arrive.
    const hostElement = useRef(null);
    const [host, setHost] = useState(null);
    const hostRef = useCallback((element) => {
        hostElement.current = element;
        setHost(element);
    }, []);
    const slides = useRef(new Map());
    const state = useRef({ pos: 0, target: 0, v: 0, raf: 0, last: 0, drag: null, swallow: false, wheelAt: 0 });
    const [range, setRange] = useState([0, 0]);
    const [target, setTarget] = useState(0);
    const last = Math.max(0, count - 1);
    const options = useRef({});
    options.current = { onTarget, onSettle, dragEnabled, last };

    const sync = useCallback(() => {
        const current = state.current;
        const element = hostElement.current;

        if (!element) {
            return;
        }
        const unit = element.clientHeight || innerHeight;
        const top = options.current.last;
        const low = clampTo(Math.floor(current.pos) - 1, 0, top), high = clampTo(Math.ceil(current.pos) + 1, 0, top);
        setRange((previous) => (previous[0] === low && previous[1] === high ? previous : [low, high]));

        for (const [index, slide] of slides.current) {
            const offset = index - current.pos, distance = Math.min(1, Math.abs(offset));
            slide.style.transform = `translate3d(0,${(offset * unit).toFixed(1)}px,0)`;
            slide.style.zIndex = String(10 - Math.round(Math.abs(offset) * 4));
            const visual = slide.querySelector(".pvis");
            const info = slide.querySelector(".pinfo");
            const dim = slide.querySelector(".pdim");

            if (visual) {
                visual.style.transform = `translate3d(0,${(-offset * unit * 0.55).toFixed(1)}px,0) scale(${(1 + distance * 0.12).toFixed(3)})`;
            }

            if (info) {
                info.style.transform = `translate3d(0,${(offset * unit * 0.28).toFixed(1)}px,0)`;
                info.style.opacity = Math.max(0, 1 - Math.abs(offset) * 1.7).toFixed(3);
            }

            if (dim) {
                dim.style.opacity = (distance * 0.75).toFixed(3);
            }

            if (index === current.target) {
                slide.removeAttribute("inert");
            } else {
                slide.setAttribute("inert", "");
            }
        }
    }, []);

    const tick = useCallback((now) => {
        const current = state.current;
        const dt = Math.min(0.034, (now - (current.last || now)) / 1000) || 0.016;
        current.last = now;

        if (!current.drag) {
            const damping = 2 * Math.sqrt(STIFFNESS);
            current.v += (-STIFFNESS * (current.pos - current.target) - damping * current.v) * dt;
            current.pos += current.v * dt;

            if (Math.abs(current.pos - current.target) < 0.002 && Math.abs(current.v) < 0.02) {
                current.pos = current.target;
                current.v = 0;
                current.raf = 0;
                current.last = 0;
                sync();
                options.current.onSettle?.(current.target);

                return;
            }
        }
        sync();
        current.raf = requestAnimationFrame(tick);
    }, [sync]);

    const kick = useCallback(() => {
        const current = state.current;

        if (!current.raf) {
            current.last = 0;
            current.raf = requestAnimationFrame(tick);
        }
    }, [tick]);

    const goTo = useCallback((index) => {
        const current = state.current;
        const next = clampTo(index, 0, options.current.last);

        if (next !== current.target) {
            current.target = next;
            setTarget(next);
            options.current.onTarget?.(next);
        }
        kick();
    }, [kick]);

    // Une page de plus ou de moins ; au bout, un rebond sans changer de page.
    const step = useCallback((delta) => {
        const current = state.current;
        const next = current.target + delta;

        if (next < 0 || next > options.current.last) {
            current.v = Math.sign(delta) * BOUNCE;
            kick();

            return false;
        }
        goTo(next);

        return true;
    }, [goTo, kick]);

    // Aller directement à une page, sans traverser celles d'entre-deux (la liste des jeux).
    const jump = useCallback((index) => {
        const current = state.current;
        const next = clampTo(index, 0, options.current.last);
        cancelAnimationFrame(current.raf);
        current.raf = 0;
        current.pos = current.target = next;
        current.v = 0;
        setTarget(next);
        sync();
        options.current.onTarget?.(next);
    }, [sync]);

    // Une référence stable par page : une nouvelle fonction à chaque rendu ferait
    // détacher et rattacher chaque page à chaque rendu.
    const slideRefs = useRef(new Map());
    const slideRef = useCallback((index) => {
        if (!slideRefs.current.has(index)) {
            slideRefs.current.set(index, (element) => {
                if (element) {
                    slides.current.set(index, element);
                } else {
                    slides.current.delete(index);
                }
            });
        }

        return slideRefs.current.get(index);
    }, []);

    // Une page qui vient d'être montée (la fenêtre a bougé, la liste a changé)
    // prend tout de suite sa place.
    useLayoutEffect(() => {
        sync();
    }, [sync, range[0], range[1], count]);

    useEffect(() => {
        const onResize = () => sync();
        window.addEventListener("resize", onResize);

        return () => {
            window.removeEventListener("resize", onResize);
            cancelAnimationFrame(state.current.raf);
        };
    }, [sync]);

    // GLISSER (le doigt, la souris) et la MOLETTE, sur l'hôte.
    useEffect(() => {
        if (!host) {
            return undefined;
        }
        const down = (event) => {
            if (event.button > 0 || !options.current.dragEnabled) {
                return;
            }
            state.current.drag = { a0: event.clientY, b0: event.clientX, from: state.current.pos, id: event.pointerId, active: false, samples: [[performance.now(), event.clientY]] };
        };
        const moved = (event) => {
            const drag = state.current.drag;

            if (!drag || event.pointerId !== drag.id) {
                return;
            }
            const delta = event.clientY - drag.a0;

            if (!drag.active) {
                if (Math.abs(delta) < 10 || Math.abs(delta) < Math.abs(event.clientX - drag.b0)) {
                    return;
                }
                drag.active = true;
                host.setPointerCapture(event.pointerId);
            }
            const top = options.current.last;
            let position = drag.from - delta / (host.clientHeight || innerHeight);

            if (position < 0) {
                position *= 0.3;
            } else if (position > top) {
                position = top + (position - top) * 0.3;
            }
            state.current.pos = position;
            drag.samples.push([performance.now(), event.clientY]);

            if (drag.samples.length > 6) {
                drag.samples.shift();
            }
            kick();
        };
        const release = (event) => {
            const drag = state.current.drag;

            if (!drag || event.pointerId !== drag.id) {
                return;
            }
            state.current.drag = null;

            if (!drag.active) {
                return;
            }
            const [t0, a0] = drag.samples[0], [t1, a1] = drag.samples[drag.samples.length - 1];
            const velocity = t1 > t0 ? -(a1 - a0) / (t1 - t0) * 1000 / (host.clientHeight || innerHeight) : 0;
            const from = Math.round(drag.from), moved = state.current.pos - drag.from;
            let next = from;

            // Lâché vite ou au-delà de 18 % : la voisine ; sinon, retour.
            if (Math.abs(velocity) > 0.6 || Math.abs(moved) > 0.18) {
                next = from + ((Math.abs(velocity) > 0.6 ? velocity : moved) > 0 ? 1 : -1);
            }
            state.current.v = velocity * 0.6;

            // Une page tournée au doigt sonne comme à la croix (libs/input.js, le son d'un déplacement).
            if (clampTo(next, 0, options.current.last) !== state.current.target) {
                playSound("move");
            }
            goTo(clampTo(next, from - 1, from + 1));
            state.current.swallow = true;
            setTimeout(() => {
                state.current.swallow = false;
            }, 60);
        };
        // UNE PAGE PAR GESTE : un pavé tactile envoie des dizaines d'événements
        // (et son inertie) ; on ne compte qu'un geste après un silence.
        const wheel = (event) => {
            if (!options.current.dragEnabled) {
                return;
            }
            const now = performance.now();
            const quiet = now - state.current.wheelAt > WHEEL_QUIET_MS;
            state.current.wheelAt = now;

            if (quiet && Math.abs(event.deltaY) >= 4 && step(event.deltaY > 0 ? 1 : -1)) {
                playSound("move");
            }
        };
        const swallowClick = (event) => {
            if (state.current.swallow) {
                event.preventDefault();
                event.stopPropagation();
            }
        };
        host.addEventListener("pointerdown", down);
        host.addEventListener("pointermove", moved);
        host.addEventListener("pointerup", release);
        host.addEventListener("pointercancel", release);
        host.addEventListener("wheel", wheel, { passive: true });
        host.addEventListener("click", swallowClick, true);

        return () => {
            host.removeEventListener("pointerdown", down);
            host.removeEventListener("pointermove", moved);
            host.removeEventListener("pointerup", release);
            host.removeEventListener("pointercancel", release);
            host.removeEventListener("wheel", wheel);
            host.removeEventListener("click", swallowClick, true);
        };
    }, [host, kick, goTo, step]);

    const visible = [];

    for (let index = range[0]; index <= Math.min(range[1], last); index++) {
        visible.push(index);
    }

    return { hostRef, slideRef, visible, target, goTo, step, jump, state };
}
