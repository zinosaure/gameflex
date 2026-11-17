// LES SONS DE LA NAVIGATION (le porteur, 2026-11-12) : un pour un déplacement, un
// pour valider ou entrer (A, ouvrir un panneau, la bibliothèque), un pour annuler ou
// sortir (B, fermer). Synthétisés par la Web Audio API — de courts bips doux, aucun
// fichier de son, rien qui appartienne à quelqu'un d'autre.
//
// Le contexte audio naît au premier geste (la règle de lecture automatique des
// navigateurs) ; le volume est bas ; un interrupteur dans les Réglages, allumé par
// défaut, gardé dans ce navigateur. Une croix tenue ne sonne pas plus d'une fois par
// 90 ms.

const KEY = "gameflex-edge-sounds";
const VOLUME = 0.06;
const MOVE_GAP_MS = 90;

// Chaque son : une ou deux notes (fréquence en Hz, durée en s), une enveloppe douce.
const SOUNDS = {
    move: [[880, 0.035]],
    enter: [[660, 0.05], [990, 0.07]],
    back: [[740, 0.05], [494, 0.07]],
};

let context = null;
let lastMove = 0;
// Le choix de cette page (le stockage peut manquer) ; `null` : celui du navigateur.
let memory = null;

export const soundsEnabled = () => {
    try {
        return localStorage.getItem(KEY) !== "off";
    } catch {
        return true;
    }
};

export const setSoundsEnabled = (on) => {
    memory = on;

    try {
        localStorage.setItem(KEY, on ? "on" : "off");
    } catch {
        // Sans stockage, le choix ne vaut que pour cette page.
    }
};

const enabled = () => (memory === null ? soundsEnabled() : memory);

// Le contexte, créé au premier geste seulement ; réveillé s'il dort.
export const wakeAudio = () => {
    const Context = window.AudioContext || window.webkitAudioContext;

    if (!Context) {
        return null;
    }

    try {
        if (!context) {
            context = new Context();
        }

        if (context.state === "suspended") {
            context.resume().catch(() => {});
        }
    } catch {
        context = null;
    }

    return context;
};

export function playSound(name) {
    if (!enabled() || !SOUNDS[name]) {
        return;
    }
    const now = performance.now();

    if (name === "move") {
        if (now - lastMove < MOVE_GAP_MS) {
            return;
        }
        lastMove = now;
    }
    const audio = wakeAudio();

    if (!audio) {
        return;
    }

    try {
        let at = audio.currentTime + 0.005;

        for (const [frequency, length] of SOUNDS[name]) {
            const oscillator = audio.createOscillator();
            const gain = audio.createGain();
            oscillator.type = "sine";
            oscillator.frequency.setValueAtTime(frequency, at);
            gain.gain.setValueAtTime(0.0001, at);
            gain.gain.exponentialRampToValueAtTime(VOLUME, at + 0.008);
            gain.gain.exponentialRampToValueAtTime(0.0001, at + length);
            oscillator.connect(gain).connect(audio.destination);
            oscillator.start(at);
            oscillator.stop(at + length + 0.02);
            at += length * 0.8;
        }
    } catch {
        // Un son qui ne part pas ne gêne rien.
    }
}
