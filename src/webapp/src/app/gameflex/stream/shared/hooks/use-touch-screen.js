import { useEffect } from "react";

// L'ÉCRAN TACTILE — la Nintendo DS, et toute console dont le core lit
// RETRO_DEVICE_POINTER.
//
// UN SEUL EXEMPLAIRE POUR LES DEUX PLATEFORMES. Le serveur et le téléphone
// font tourner le même moteur, mais le doigt ne leur arrive pas par le même
// chemin ; la question qu'on leur pose, elle, est la même : « le doigt est
// là ». Ce hook ne fait que la poser ; c'est le hook de plateforme qui fournit
// `send` et sait à qui parler.
//
// ON N'ESSAIE PAS DE TROUVER L'ÉCRAN DU BAS. Les coordonnées portent sur
// TOUTE l'image, dans la convention de libretro (-1..1, centre à 0), et c'est
// le core qui sait où se trouve sa zone tactile selon la disposition qu'il
// applique. Deviner à sa place voudrait dire refaire ce calcul pour chaque
// core, chaque disposition, et se tromper à chaque changement de l'un ou de
// l'autre. Un stylet posé hors de la zone reste hors d'elle, ce qui est le
// comportement de la vraie console.
//
// Les écouteurs sont posés sur le <video> lui-même, pas sur un calque
// au-dessus, pour une raison précise : `offsetX`/`offsetY` sont exprimés dans
// le repère PROPRE de l'élément visé. L'écran de jeu est parfois tourné de
// 90° (le mode paysage forcé sur un téléphone tenu droit), et une mesure
// prise dans le repère de la page y serait fausse d'un quart de tour.
//
// screenRef — le manche de GatewayWebRTC ; son `videoElement` est un getter,
// donc toujours à jour.
// enabled — cette console a un écran tactile ET on tient la manette.
// send(x, y, pressed) — le dernier mètre, propre à la plateforme.
export function useTouchScreen({ screenRef, enabled, send }) {
    useEffect(() => {
        if (!enabled || !send) {
            return undefined;
        }

        let video = null;
        let finder = null;
        let detach = null;
        // Le doigt en cours. `id` est celui du pointeur, pour ignorer un
        // second doigt sans se laisser voler le premier.
        const held = { id: null, x: 0, y: 0 };

        // Où le doigt tombe dans l'image, en coordonnées libretro.
        //
        // Le <video> est en `object-fit: contain` : l'image est centrée dans
        // l'élément avec des bandes noires, et un clic sur une bande n'est
        // pas un clic sur l'image. Sans ce calcul, tout serait décalé dès que
        // les proportions du jeu et celles de la fenêtre diffèrent — c'est-à-
        // dire presque toujours, la DS étant deux écrans empilés.
        const locate = (event) => {
            const boxWidth = video.clientWidth;
            const boxHeight = video.clientHeight;
            const frameWidth = video.videoWidth;
            const frameHeight = video.videoHeight;
            if (!boxWidth || !boxHeight || !frameWidth || !frameHeight) {
                return null;
            }
            const scale = Math.min(boxWidth / frameWidth, boxHeight / frameHeight);
            const drawnWidth = frameWidth * scale;
            const drawnHeight = frameHeight * scale;
            const across = (event.offsetX - (boxWidth - drawnWidth) / 2) / drawnWidth;
            const down = (event.offsetY - (boxHeight - drawnHeight) / 2) / drawnHeight;

            return {
                x: Math.max(-1, Math.min(1, across * 2 - 1)),
                y: Math.max(-1, Math.min(1, down * 2 - 1)),
                inside: across >= 0 && across <= 1 && down >= 0 && down <= 1,
            };
        };

        const onDown = (event) => {
            if (held.id !== null) {
                return;
            }
            const at = locate(event);
            // Une bande noire n'est pas l'écran : ne rien envoyer, et ne pas
            // capturer le pointeur non plus, pour que le reste de la page
            // continue de le recevoir.
            if (!at || !at.inside) {
                return;
            }
            held.id = event.pointerId;
            held.x = at.x;
            held.y = at.y;
            // La capture garde le doigt à nous même s'il sort du cadre en
            // glissant — sans elle, un relâchement hors de l'élément ne nous
            // parvient jamais et le stylet reste posé pour toujours.
            try {
                video.setPointerCapture(event.pointerId);
            } catch {
                // Un navigateur qui refuse la capture ne casse rien : on perd
                // seulement les relâchements hors cadre, que pointercancel
                // rattrape la plupart du temps.
            }
            event.preventDefault();
            send(at.x, at.y, true);
        };

        const onMove = (event) => {
            if (held.id !== event.pointerId) {
                return;
            }
            const at = locate(event);
            if (!at) {
                return;
            }
            // Volontairement PAS de test `inside` ici : un doigt qui glisse au
            // bord reste posé, borné au bord — comme un stylet qu'on pousse
            // contre le cadre de la console. Le lever parce qu'il sort d'un
            // pixel serait une rupture de trait au pire moment.
            held.x = at.x;
            held.y = at.y;
            event.preventDefault();
            send(at.x, at.y, true);
        };

        const onUp = (event) => {
            if (held.id !== event.pointerId) {
                return;
            }
            held.id = null;
            try {
                video.releasePointerCapture(event.pointerId);
            } catch {
                // Déjà relâchée (pointercancel l'a fait) — rien à faire.
            }
            event.preventDefault();
            // La DERNIÈRE position, pas le centre : un core lit souvent X/Y
            // une image après PRESSED, et remettre à zéro ici ferait un clic
            // au milieu de l'écran juste avant de lever le doigt.
            send(held.x, held.y, false);
        };

        const attach = (element) => {
            video = element;
            // Sans ça, le navigateur préempte le geste pour faire défiler ou
            // zoomer, et le stylet ne reçoit qu'un `pointercancel`.
            const previousTouchAction = video.style.touchAction;
            video.style.touchAction = "none";
            video.addEventListener("pointerdown", onDown);
            video.addEventListener("pointermove", onMove);
            video.addEventListener("pointerup", onUp);
            video.addEventListener("pointercancel", onUp);
            detach = () => {
                video.style.touchAction = previousTouchAction;
                video.removeEventListener("pointerdown", onDown);
                video.removeEventListener("pointermove", onMove);
                video.removeEventListener("pointerup", onUp);
                video.removeEventListener("pointercancel", onUp);
                // Ne jamais laisser un stylet posé derrière soi : le core
                // garderait l'appui pour le restant de la partie.
                if (held.id !== null) {
                    send(held.x, held.y, false);
                }
            };
        };

        // Le <video> existe dès le premier rendu de GatewayWebRTC, mais ce
        // hook ne le lit qu'à travers un ref : selon l'ordre de montage, il
        // peut ne pas encore être là. On regarde jusqu'à ce qu'il le soit,
        // plutôt que de supposer un ordre qu'un remaniement changerait sans
        // prévenir.
        const look = () => {
            const element = screenRef?.current?.videoElement;
            if (element) {
                attach(element);

                return;
            }
            finder = setTimeout(look, 200);
        };
        look();

        return () => {
            clearTimeout(finder);
            detach?.();
        };
    }, [enabled, send, screenRef]);
}
