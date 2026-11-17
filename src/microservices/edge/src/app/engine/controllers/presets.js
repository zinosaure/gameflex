// Utility function to replace name, icon, and text properties in controller presets based on a replacements map.
const replace_nameicontexts = (presets, replacements = { name: { name: null, icon: null, text: null } }) => {
    return presets.map(preset => {
        let updatedPreset = { ...preset };

        Object.entries(replacements).forEach(([name, value]) => {
            if (name && updatedPreset.name === name) {
                updatedPreset = { ...updatedPreset, ...value };
            }
        });

        return updatedPreset;
    });
};

// Virtual-controller layouts — pure data, no React: which buttons a given
// preset shows, where each one sits (portrait `x`/`y`, landscape `xl`), how
// big it is, and what it displays. Consumed by ControllerLayout.jsx, which
// owns the rendering and the input plumbing.
//
// A layout a user has repositioned themselves is persisted as a FULL
// snapshot of one of these arrays (see libs/profileApi.js's
// saveButtonPositioning, and ControllerLayout's own loading effect) and is
// never re-merged with the preset. Editing a preset therefore only affects
// players who have no saved layout for it — and renaming a button silently
// breaks the saved ones, since callbacks are keyed by `name`.
const DEFAULT_BUTTON_SIZE = 50; // pixels
const SMALL_BUTTON_SIZE = 0.85; // multiplier for small buttons
const XSMALL_BUTTON_SIZE = 0.7; // multiplier for small buttons
const LARGE_BUTTON_SIZE = 1.2; // multiplier for large buttons
const XLARGE_BUTTON_SIZE = 1.35; // multiplier for extra large buttons
const XXLARGE_BUTTON_SIZE = 1.5; // multiplier for extra large buttons
const TOUCHPAD_LANDSCAPE_SIZE = 180; // pixels : le pavé tactile de la manette PS4/PS5, en paysage (360 × 180)
const STICK_ANALOG_SIZE = 85; // pixels
const N64_C_LIFT_LANDSCAPE = 0.11; // fraction de la colonne : les quatre C de la N64, remontés en paysage

// LES TOUCHES REMONTENT, LES STICKS RESTENT EN BAS (#758 ; le porteur, 2026-11-17 : « remonter les touches un peu plus
// haut et donner plus d'espace aux stick. Tu laisses les sticks vers le bas et les touches tu remontes »). La croix et
// les faces étaient centrées à 0,6 de la colonne en portrait, 0,5 en paysage : leur bas touchait presque le haut des
// sticks (8 px en portrait, 10 en paysage, mesurés sur une partie N64). Elles montent à 0,48 et 0,44 ; Select / Start
// remontent aussi ; en paysage, les gâchettes montent avec elles (L2 / R2 un peu plus : elles sont au-dessus de la croix
// et des faces), sans sortir du haut de la colonne ; en portrait, elles sont déjà en haut. Toutes les manettes partagent
// ces constantes et ces groupes : elles changent ensemble. Une disposition enregistrée par le joueur est une copie
// complète (voir plus haut) : elle ne bouge pas.
const BUTTON_MIDDLE_POSITION_X = 0.5; // middle position for buttons (both x and y)
const BUTTON_MIDDLE_POSITION_Y = 0.48; // middle position for buttons (both x and y)
const BUTTON_MIDDLE_POSITION_X_LANDSCAPE = 0.5; // middle position for buttons in landscape mode (both x and y)
const BUTTON_MIDDLE_POSITION_Y_LANDSCAPE = 0.44; // middle position for buttons in landscape mode (both x and y)
const DPAD_POSITIONS = [
    {
        name: "DPAD_UP",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X + 0.05,
        y: BUTTON_MIDDLE_POSITION_Y - 0.2,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.225,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.175,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
        },
        icon: "fa-solid fa-arrow-up fs-5",
        group: "dpad",
    },
    {
        name: "DPAD_LEFT",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X - 0.2,
        y: BUTTON_MIDDLE_POSITION_Y + -0.075,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE - 0.05,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.025,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-arrow-left fs-5",
        group: "dpad",
    },
    {
        name: "DPAD_RIGHT",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X + 0.3,
        y: BUTTON_MIDDLE_POSITION_Y + -0.075,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.5,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.025,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-arrow-right fs-5",
        group: "dpad",
    },
    {
        name: "DPAD_DOWN",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X + 0.05,
        y: BUTTON_MIDDLE_POSITION_Y + 0.05,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.225,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + 0.125,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-arrow-down fs-5",
        group: "dpad",
    },
    {
        name: "DPAD_UP_LEFT",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X - 0.17,
        y: BUTTON_MIDDLE_POSITION_Y - 0.19,
        size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE - 0.03,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.155,
            size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE
        },
        icon: "bi bi-arrow-up-left fs-6",
        diagonal: true,
        group: "dpad",
    },
    {
        name: "DPAD_UP_RIGHT",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X + 0.27,
        y: BUTTON_MIDDLE_POSITION_Y - 0.19,
        size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.48,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.155,
            size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE
        },
        icon: "bi bi-arrow-up-right fs-6",
        diagonal: true,
        group: "dpad",
    },
    {
        name: "DPAD_DOWN_LEFT",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X - 0.17,
        y: BUTTON_MIDDLE_POSITION_Y + 0.04,
        size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE - 0.03,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + 0.125,
            size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE
        },
        icon: "bi bi-arrow-down-left fs-6",
        diagonal: true,
        group: "dpad",
    },
    {
        name: "DPAD_DOWN_RIGHT",
        column: "left",
        x: BUTTON_MIDDLE_POSITION_X + 0.27,
        y: BUTTON_MIDDLE_POSITION_Y + 0.04,
        size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.48,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + 0.125,
            size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE
        },
        icon: "bi bi-arrow-down-right fs-6",
        diagonal: true,
        group: "dpad",
    },
];

// Presets for different controller types, defining button positions and sizes
// These presets are used to render the virtual controller overlay for different gamepad layouts

// Groupes de boutons partagés — mêmes définitions, réutilisées telles
// quelles par plusieurs manettes. Ils prolongent DPAD_POSITIONS ci-dessus :
// une constante par groupe, assemblée dans l'ordre de la manette.
//
// Les épaules et les sticks sont séparés gauche/droite parce que les
// manettes les déclarent entrelacés (L2, L1, … R2, R1) — garder les deux
// côtés distincts est ce qui permet de substituer sans changer l'ordre
// du tableau, dont dépend l'ordre de rendu (voir ColumnZone).

const PS_FACE_BUTTONS = [
    {
        name: "TRIANGLE",
        column: "right",
        x: BUTTON_MIDDLE_POSITION_X + -0.05,
        y: BUTTON_MIDDLE_POSITION_Y + -0.2,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + -0.225,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.175,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "bi bi-triangle fs-5",
        group: "buttons",
    },
    {
        name: "SQUARE",
        column: "right",
        x: BUTTON_MIDDLE_POSITION_X + -0.3,
        y: BUTTON_MIDDLE_POSITION_Y + -0.075,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE - 0.5,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.025,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-regular fa-square fs-5",
        group: "buttons",
    },
    {
        name: "CIRCLE",
        column: "right",
        x: BUTTON_MIDDLE_POSITION_X + 0.2,
        y: BUTTON_MIDDLE_POSITION_Y + -0.075,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.05,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.025,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-regular fa-circle fs-5",
        group: "buttons",
    },
    {
        name: "CROSS",
        column: "right",
        x: BUTTON_MIDDLE_POSITION_X + -0.05,
        y: BUTTON_MIDDLE_POSITION_Y + 0.05,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + -0.225,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + 0.125,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-xmark fs-3",
        group: "buttons",
    },
];

const DIAMOND_FACE_BUTTONS = replace_nameicontexts(PS_FACE_BUTTONS, {
    "TRIANGLE": {
        name: "X",
        icon: "fa-solid fa-x fs-5"
    },
    "SQUARE": {
        name: "Y",
        icon: "fa-solid fa-y fs-5"
    },
    "CIRCLE": {
        name: "A",
        icon: "fa-solid fa-a fs-5"
    },
    "CROSS": {
        name: "B",
        icon: "fa-solid fa-b fs-5"
    }
});

const SELECT_START_BUTTONS = [
    {
        name: "SELECT",
        column: "left",
        x: 0.9,
        y: 0.15,
        size: DEFAULT_BUTTON_SIZE * SMALL_BUTTON_SIZE,
        xl: {
            x: 1,
            y: 0.06,
            size: DEFAULT_BUTTON_SIZE * SMALL_BUTTON_SIZE
        },
        icon: "fa-solid fa-stop fs-6",
        group: "controls",
    },
    {
        name: "START",
        column: "right",
        x: 0.1,
        y: 0.15,
        size: DEFAULT_BUTTON_SIZE * SMALL_BUTTON_SIZE,
        xl: {
            x: 0,
            y: 0.06,
            size: DEFAULT_BUTTON_SIZE * SMALL_BUTTON_SIZE
        },
        icon: "fa-solid fa-play fs-6",
        group: "controls",
    },
];

const PS_LEFT_SHOULDER_BUTTONS = [
    {
        name: "L2",
        column: "left",
        x: 0.55,
        y: 0,
        size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
        xl: {
            x: 0.6,
            y: 0.04,
            size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE
        },
        text: "L2",
        group: "left_shoulders",
    },
    {
        name: "L1",
        column: "left",
        x: 0.2,
        y: 0.1,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: 0.225,
            y: 0.16,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        text: "L1",
        group: "left_shoulders",
    },
];

const PS_RIGHT_SHOULDER_BUTTONS = [
    {
        name: "R2",
        column: "right",
        x: 0.45,
        y: 0,
        size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
        xl: {
            x: 0.4,
            y: 0.04,
            size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE
        },
        text: "R2",
        group: "right_shoulders",
    },
    {
        name: "R1",
        column: "right",
        x: 0.8,
        y: 0.1,
        size: DEFAULT_BUTTON_SIZE,
        xl: {
            x: 0.775,
            y: 0.16,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        text: "R1",
        group: "right_shoulders",
    },
];

const AB_FACE_BUTTONS = [
    {
        name: "A",
        column: "right",
        x: BUTTON_MIDDLE_POSITION_X + 0.20,
        y: BUTTON_MIDDLE_POSITION_Y + -0.05,
        size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + 0.20,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.05,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-a fs-5",
        group: "buttons",
    },
    {
        name: "B",
        column: "right",
        x: BUTTON_MIDDLE_POSITION_X + -0.3,
        y: BUTTON_MIDDLE_POSITION_Y + -0.05,
        size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
        xl: {
            x: BUTTON_MIDDLE_POSITION_X_LANDSCAPE + -0.3,
            y: BUTTON_MIDDLE_POSITION_Y_LANDSCAPE + -0.05,
            size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-b fs-5",
        group: "buttons",
    },
];

const LEFT_STICK_POSITION = [
    {
        name: "LEFT_STICK",
        column: "left",
        x: 0.7,
        y: 0.9,
        size: STICK_ANALOG_SIZE,
        xl: {
            x: 0.5,
            y: 0.9,
            size: STICK_ANALOG_SIZE
        },
        analog: true,
    },
];

const RIGHT_STICK_POSITION = [
    {
        name: "RIGHT_STICK",
        column: "right",
        x: 0.3,
        y: 0.9,
        size: STICK_ANALOG_SIZE,
        xl: {
            x: 0.5,
            y: 0.9,
            size: STICK_ANALOG_SIZE
        },
        analog: true,
    },
];

const LEFT_STICK_CLICK = [
    {
        name: "L3",
        column: "left",
        x: 0.2,
        y: 0.75,
        size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE,
        xl: {
            x: 0.225,
            y: 0.675,
            size: DEFAULT_BUTTON_SIZE * SMALL_BUTTON_SIZE
        },
        text: "L3",
    },
];

const RIGHT_STICK_CLICK = [
    {
        name: "R3",
        column: "right",
        x: 0.8,
        y: 0.75,
        size: DEFAULT_BUTTON_SIZE * XSMALL_BUTTON_SIZE,
        xl: {
            x: 0.775,
            y: 0.675,
            size: DEFAULT_BUTTON_SIZE * SMALL_BUTTON_SIZE
        },
        text: "R3",
    },
];

const LEFT_SHOULDER_BUTTON = [
    {
        name: "L",
        column: "left",
        x: 0.3,
        y: 0,
        size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE,
        xl: {
            x: 0.325,
            y: 0.06,
            size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-l fs-5",
    },
];

const RIGHT_SHOULDER_BUTTON = [
    {
        name: "R",
        column: "right",
        x: 0.7,
        y: 0,
        size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE,
        xl: {
            x: 0.675,
            y: 0.06,
            size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE
        },
        icon: "fa-solid fa-r fs-5",
    },
];

// ORDER MATTERS. The settings page builds its picker with
// Object.entries(...), so the FIRST entry is what a fresh picker shows — and
// what the TEST button then streams. It began with the digital ps1 pad, whose
// label is literally "PS1 (v1)", which is why the DualShock never appeared by
// default however often the console→preset map was corrected (reported three
// times: 2026-08-24, 2026-08-25). The map said dualshock; the list said v1;
// the list is what the eye sees.
export const CONTROLLER_PRESETS = {
    ps1: {
        label: "PS1 (v1)",
        rumble: false,
        buttons: [
            ...PS_LEFT_SHOULDER_BUTTONS,
            ...SELECT_START_BUTTONS,
            ...PS_RIGHT_SHOULDER_BUTTONS,
            ...PS_FACE_BUTTONS,
            ...DPAD_POSITIONS,
            ...LEFT_STICK_POSITION,
        ],
    },
    dualshock: {
        label: "PS1/2 (DualShock)",
        rumble: true,
        buttons: [
            ...LEFT_STICK_CLICK,
            ...PS_LEFT_SHOULDER_BUTTONS,
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...RIGHT_STICK_CLICK,
            ...PS_RIGHT_SHOULDER_BUTTONS,
            ...PS_FACE_BUTTONS,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
    dualshock3: {
        label: "PS3 (DualShock 3)",
        rumble: true,
        buttons: [
            {
                name: "PS",
                column: "center",
                x: 0.5,
                y: 0.7,
                size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
                xl: {
                    x: 0.5,
                    y: 0.7,
                    size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE
                },
                icon: "bi bi-playstation fs-4",
            },
            ...LEFT_STICK_CLICK,
            ...PS_LEFT_SHOULDER_BUTTONS,
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...RIGHT_STICK_CLICK,
            ...PS_RIGHT_SHOULDER_BUTTONS,
            ...PS_FACE_BUTTONS,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
    dualsense: {
        label: "PS4 (DualShock 4) / PS5 (DualSense)",
        rumble: true,
        buttons: [
            {
                // EN PAYSAGE, PRESQUE TOUT LE MILIEU DE L'ÉCRAN (#758 ; le porteur, 2026-11-17, deux dessins : « Il faut
                // que ça soit à la taille comme j'ai dessiné. En mode landscape seulement », puis « Encore plus gros […]
                // ça doit prendre presque toute l'écran du milieu ») : 400 × 200, puis « réduire la taille du touchpad de
                // 10% » : 360 × 180 (la surface fait deux fois sa taille en largeur, TouchpadSurface.jsx), entre la
                // croix et les faces d'un S25 tenu à l'horizontale ; centré à 0,27 de la colonne du milieu, au-dessus du
                // bouton PS et de la zone du stick flottant.
                //
                // EN PORTRAIT, SA TAILLE, PLUS HAUT (le porteur : « La taille était bonne c'était le positionnement de
                // l'axe Y qui n'était pas bon ») : 100 × 50, centré à −0,06 de la colonne au lieu de 0 — son bas passe
                // au-dessus de « ••• » et « ≡ ».
                name: "TOUCHPAD",
                column: "center",
                x: 0.5,
                y: -0.06,
                size: DEFAULT_BUTTON_SIZE,
                xl: {
                    x: 0.5,
                    y: 0.27,
                    size: TOUCHPAD_LANDSCAPE_SIZE
                },
                icon: "bi bi-square fs-1",
            },
            {
                name: "PS",
                column: "center",
                x: 0.5,
                y: 0.7,
                size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
                xl: {
                    x: 0.5,
                    y: 0.9,
                    size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
                },
                icon: "bi bi-playstation fs-4",
            },
            ...LEFT_STICK_CLICK,
            ...PS_LEFT_SHOULDER_BUTTONS,
            ...LEFT_STICK_POSITION,
            ...replace_nameicontexts(SELECT_START_BUTTONS, {
                "SELECT": {
                    name: "SHARE",
                    icon: "bi bi-three-dots fs-5",
                },
                "START": {
                    name: "OPTIONS",
                    icon: "fa-solid fa-bars fs-5",
                }
            }),
            ...RIGHT_STICK_CLICK,
            ...PS_RIGHT_SHOULDER_BUTTONS,
            ...PS_FACE_BUTTONS,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
    psp: {
        label: "PSP/PSVita",
        rumble: false,
        buttons: [
            ...LEFT_SHOULDER_BUTTON,
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...RIGHT_SHOULDER_BUTTON,
            ...PS_FACE_BUTTONS,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
    nes: {
        label: "Nintendo Entertainment System (NES)",
        rumble: false,
        buttons: [
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...AB_FACE_BUTTONS,
            ...DPAD_POSITIONS,
        ],
    },
    snes: {
        label: "Super Nintendo Entertainment System (SNES)",
        rumble: false,
        buttons: [
            ...LEFT_SHOULDER_BUTTON,
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...RIGHT_SHOULDER_BUTTON,
            ...DIAMOND_FACE_BUTTONS,
            ...DPAD_POSITIONS,
        ],
    },
    gba: {
        label: "Game Boy Advance",
        rumble: false,
        buttons: [
            ...LEFT_SHOULDER_BUTTON,
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...RIGHT_SHOULDER_BUTTON,
            ...AB_FACE_BUTTONS,
            ...DPAD_POSITIONS,
        ],
    },
    xbox: {
        label: "Xbox/Xbox 360",
        rumble: true,
        buttons: [
            ...replace_nameicontexts(PS_LEFT_SHOULDER_BUTTONS, {
                "L2": {
                    name: "LB",
                    text: "LB",
                },
                "L1": {
                    name: "LT",
                    text: "LT",
                }
            }),
            ...replace_nameicontexts(PS_RIGHT_SHOULDER_BUTTONS, {
                "R2": {
                    name: "RB",
                    text: "RB",
                },
                "R1": {
                    name: "RT",
                    text: "RT",
                }
            }),
            ...replace_nameicontexts(LEFT_STICK_CLICK, {
                "L3": {
                    name: "LS",
                    text: "LS",
                }
            }),
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...replace_nameicontexts(RIGHT_STICK_CLICK, {
                "R3": {
                    name: "RS",
                    text: "RS",
                }
            }),
            ...DIAMOND_FACE_BUTTONS,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
    joycon: {
        label: "Nintendo Switch",
        rumble: true,
        buttons: [
            ...replace_nameicontexts(PS_LEFT_SHOULDER_BUTTONS, {
                "L2": {
                    name: "ZL",
                    text: "ZL",
                },
                "L1": {
                    name: "L",
                    text: "L",
                }
            }),
            ...replace_nameicontexts(PS_RIGHT_SHOULDER_BUTTONS, {
                "R2": {
                    name: "ZR",
                    text: "ZR",
                },
                "R1": {
                    name: "R",
                    text: "R",
                }
            }),
            ...replace_nameicontexts(SELECT_START_BUTTONS, {
                "SELECT": {
                    name: "MINUS",
                    icon: "fa-solid fa-minus fs-6",
                },
                "START": {
                    name: "PLUS",
                    icon: "fa-solid fa-plus fs-6",
                }
            }),
            // L'APPUI SUR LES STICKS (#758 ; le porteur, 2026-11-17 : « sur Nintendo Switch et Switch 2, apparemment il
            // manque les boutons (équivalent) L3 et R3 ») : les petits ronds des manettes PlayStation, aux mêmes places
            // — la croix et les faces sont les mêmes. Toute la chaîne existait déjà, seul le dessin manquait : L3 / R3 →
            // `lstick` / `rstick` (streamex/controllers/NintendoSwitch.jsx) → gp:L3 / gp:R3 (profiles/switch.json,
            // switch2.json) → les bits L3 / R3 du serveur cemuhook (shared/cemuhook), liés à button_lstick /
            // button_rstick dans l'adapter.json d'Eden, de Citron et de Lichii ; au téléphone, les bits libretro 14 / 15.
            ...LEFT_STICK_CLICK,
            ...LEFT_STICK_POSITION,
            ...DIAMOND_FACE_BUTTONS,
            ...RIGHT_STICK_CLICK,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
    n64: {
        label: "Nintendo 64/GameCube",
        rumble: true,
        buttons: [
            {
                // LE NOM EST LA CLÉ LIBRETRO, PAS L'ÉTIQUETTE.
                //
                // `name` est ce que le hook traduit en indice RetroPad
                // (RETRO_BUTTON_INDEX, Controllers.jsx) ; le joueur, lui, ne
                // voit que `text`/`icon`. Écrit « Z », ce bouton ne
                // correspondait à aucune clé : RETRO_BUTTON_INDEX["Z"] vaut
                // undefined, et l'appui partait dans le vide — la visée
                // d'Ocarina, entre autres (2026-09-08).
                //
                // mupen64plus_next place le Z de la N64 sur L2. L'étiquette
                // reste « Z », qui est ce que le joueur cherche.
                name: "L2",
                column: "center",
                x: 0.5,
                y: 0,
                size: DEFAULT_BUTTON_SIZE * XXLARGE_BUTTON_SIZE,
                xl: {
                    x: 0.5,
                    y: 0.45,
                    size: DEFAULT_BUTTON_SIZE * XXLARGE_BUTTON_SIZE
                },
                text: "Z",
            },
            {
                name: "START",
                column: "center",
                x: 0.5,
                y: 0.7,
                size: DEFAULT_BUTTON_SIZE * XLARGE_BUTTON_SIZE,
                xl: {
                    x: 0.5,
                    y: 0.7,
                    size: DEFAULT_BUTTON_SIZE * XXLARGE_BUTTON_SIZE
                },
                icon: "fa-solid fa-play fs-6",
            },
            ...LEFT_STICK_POSITION,
            ...LEFT_SHOULDER_BUTTON,
            ...RIGHT_SHOULDER_BUTTON,
            // EN PAYSAGE, LES QUATRE C REMONTENT (le porteur, 2026-11-17 : « Remonte les 4 C pour N64 ») de 0,11 de la
            // colonne : A et B se rangent alors côte à côte au-dessus du bouton du menu, le bas du C du bas 10 px
            // au-dessus d'eux (892 × 412). Le C du haut reste à 6 px de R. Le portrait ne change pas.
            ...replace_nameicontexts(PS_FACE_BUTTONS, {
                "TRIANGLE": {
                    name: "C_UP",
                    icon: "fa-solid fa-arrow-up fs-6",
                },
                "SQUARE": {
                    name: "C_LEFT",
                    icon: "fa-solid fa-arrow-left fs-6",
                },
                "CIRCLE": {
                    name: "C_RIGHT",
                    icon: "fa-solid fa-arrow-right fs-6",
                },
                "CROSS": {
                    name: "C_DOWN",
                    icon: "fa-solid fa-arrow-down fs-6",
                }
            }).map((button) => ({ ...button, xl: { ...button.xl, y: button.xl.y - N64_C_LIFT_LANDSCAPE } })),
            // EN PAYSAGE, A ET B REMONTENT (#758 ; le porteur, 2026-11-17 : « il faut remonter les touches A et B de
            // N64 en landscape ») : le bouton du menu, dans le coin en bas à droite (24 px du bas, 12 de la droite),
            // couvrait le A. A et B montent côte à côte juste au-dessus de lui, 8 px d'écart (892 × 412 : leur bas à
            // 329, le haut du menu à 337) ; les quatre C ont remonté pour leur faire place. Le portrait ne change pas.
            {
                name: "B",
                column: "right",
                x: 0.25,
                y: 0.9,
                size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
                xl: {
                    x: 0.25,
                    y: 0.69,
                    size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
                },
                icon: "fa-solid fa-b fs-5",
            },
            {
                name: "A",
                column: "right",
                x: 0.7,
                y: 0.9,
                size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE,
                xl: {
                    x: 0.7,
                    y: 0.69,
                    size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
                },
                icon: "fa-solid fa-a fs-5",
            },
            ...DPAD_POSITIONS,
        ],
    },
    genesis: {
        label: "Sega Genesis / Mega Drive",
        rumble: false,
        buttons: [
            ...LEFT_STICK_POSITION,
            {
                name: "START",
                column: "center",
                x: 0.5,
                y: 0.65,
                size: DEFAULT_BUTTON_SIZE,
                xl: {
                    x: 0.5,
                    y: 0.65,
                    size: DEFAULT_BUTTON_SIZE * LARGE_BUTTON_SIZE
                },
                icon: "fa-solid fa-play fs-6",
            },
            // MÊME PIÈGE QUE LE Z DE LA N64 : renommer `name` coupe le fil.
            //
            // Ces deux gâchettes portaient name:"Z" et name:"C" — aucune des
            // deux n'est une clé de RETRO_BUTTON_INDEX, donc aucune des deux
            // n'arrivait au cœur. Seules l'icône et le texte doivent changer.
            //
            // La table de genesis_plus_gx, pour choisir les étiquettes en
            // connaissance de cause :
            //
            //     Genesis A → RetroPad Y      Genesis X → RetroPad L
            //     Genesis B → RetroPad B      Genesis Y → RetroPad X
            //     Genesis C → RetroPad A      Genesis Z → RetroPad R
            //
            // Donc ces deux positions SONT, côté cœur, le X (gauche) et le Z
            // (droite) de la Mega Drive — pas le Z et le C. Les icônes le
            // disent maintenant. Le C, lui, est le bouton A de la façade.
            ...replace_nameicontexts(LEFT_SHOULDER_BUTTON, {
                "L": {
                    icon: "fa-solid fa-x fs-5",
                },
            }),
            ...replace_nameicontexts(RIGHT_SHOULDER_BUTTON, {
                "R": {
                    icon: "fa-solid fa-z fs-5",
                },
            }),
            ...DIAMOND_FACE_BUTTONS,
            ...DPAD_POSITIONS,
        ],
    },
    default: {
        label: "GameFlex's Gamepad",
        rumble: false,
        buttons: [
            ...LEFT_SHOULDER_BUTTON,
            ...LEFT_STICK_POSITION,
            ...SELECT_START_BUTTONS,
            ...RIGHT_SHOULDER_BUTTON,
            ...DIAMOND_FACE_BUTTONS,
            ...RIGHT_STICK_POSITION,
            ...DPAD_POSITIONS,
        ],
    },
};
