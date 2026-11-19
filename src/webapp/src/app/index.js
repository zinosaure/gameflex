import React, {
	Suspense,
	useEffect,
	useMemo,
	useContext,
} from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";

import "bootstrap/dist/css/bootstrap.min.css";
import "bootstrap-icons/font/bootstrap-icons.css";
import "@fortawesome/fontawesome-free/css/all.min.css";

import $ from "jquery";
import Popper from "popper.js";
window.$ = window.jQuery = $;
window.popperjs = Popper;

import Skeleton from "./skeleton/skeleton.js";
import "./css-js/app.css";
import "./css-js/app.js";

import { AuthProvider, useAuth } from "./gameflex/profile/auth/AuthContext.jsx";
import { useAutoMapGamepads } from "./gameflex/hooks/use-auto-map-gamepads.js";
import Login from "./gameflex/profile/auth/Login.jsx";
import InvitePage from "./gameflex/profile/auth/InvitePage.jsx";
import { can, loadHostCapabilities } from "./gameflex/libs/capabilities.js";
import { applyScreenOrientation } from "./gameflex/libs/screenOrientation.js";
import { deviceCancelDownload, preloadLocalLibrary } from "./gameflex/libs/localLibrary.js";
import {
	LoadingScreen,
	OffCanvas,
	ListElement,
	DownloadRing,
} from "./gameflex/shared/templates/Essentials.jsx";
import { useDownloadProgress } from "./gameflex/libs/deviceStream.js";
import { formatSize } from "./gameflex/profile/settings/admin/api/library.js";
import {
	PresenceProvider,
	usePresence,
} from "./gameflex/contexts/PresenceContext.jsx";
import {
	ConsolesProvider,
	useConsoles,
	useConsolesLoaded,
} from "./gameflex/contexts/ConsolesContext.jsx";
import { allows } from "./gameflex/libs/consoleGrants.js";
import {
	SessionPoolProvider,
	useSessionPool,
	hasOpenSlot,
} from "./gameflex/libs/SessionPool.js";
import {
	buildStreamingUrl,
} from "./gameflex/libs/sessionLinks.js";

// Shared base — only the "Retro Gaming" menu section's console list is
// dynamic (filtered by which consoles actually have ROMs, see
// buildMenuItems/App below); everything else here never changes at runtime.
const baseInit = {
	name: "GameFlex",
	themeMode: "dark",
	homePage: {
		to: "/",
		component: "gameflex.Dashboard",
	},
	languages: [
		{ value: "en", label: "🇬🇧 - English" },
		{ value: "fr", label: "🇫🇷 - Français" },
	],
	// LE DÉPÔT ET LE SOUTIEN, AU PIED DE CHAQUE PAGE (le porteur, 2026-10-25,
	// avant d'ouvrir une partie du code) : où trouver GameFlex, et comment
	// l'aider. Des noms propres : ils ne se traduisent pas.
	footerLinks: [
		{ href: "https://github.com/zinosaure/gameflex", icon: "bi bi-github", label: "GitHub" },
		{ href: "https://buymeacoffee.com/zinosaure", icon: "bi bi-cup-hot-fill", label: "Buy me a coffee" },
	],
};

// LE MENU LISTE TOUTES LES CONSOLES ACTIVES — sur le serveur comme sur le
// téléphone (décidé par le porteur le 2026-10-11) : « même si une console n'a
// pas de jeux localement, on le liste dans le menu ».
//
// La raison : les jeux ne sont plus seulement sur le disque. Ils arrivent d'un
// serveur fédéré, d'un partage Samba, WebDAV ou kDrive — une console sans jeu
// local en a peut-être ailleurs, et c'est sa page qui le dit. Le menu se
// construisait sur un COMPTE de jeux (et, sur le téléphone, sur la liste des
// consoles fédérées) : des entrées qui apparaissaient et disparaissaient au
// gré des chargements, un routeur qui devait attendre ces réponses, et une
// console invisible tant que son premier jeu distant n'était pas compté.
//
// « Active », c'est `Enabled` dans le registre (shared/consoles) : la liste que
// sert l'hôte n'en contient pas d'autres. Les trois sections — consoles
// rétro, streamex, jeu à distance — les prennent toutes. Une section qui n'a
// rien à montrer disparaît, titre compris.
// PAR ORDRE ALPHABÉTIQUE, dans chaque section (demandé par le porteur le
// 2026-10-11) : le registre range les consoles par constructeur, et le menu
// les montrait dans cet ordre-là. `numeric` met « PlayStation 2 » avant
// « PlayStation Portable » et après « PlayStation 1 ».
//
// L'ANNÉE en badge, à droite (demandé le même jour) : celle de la première
// sortie, que le registre porte (`year`). Pas dans le libellé — plusieurs
// noms ont déjà leurs parenthèses (« Nintendo Ent. System (NES) »). Le tri,
// lui, reste alphabétique : « par année, ça ne rapporte pas grand-chose ».
const byName = (a, b) => a.label.localeCompare(b.label, undefined, { numeric: true, sensitivity: "base" });

// `grants` — les consoles du compte (chantier 20). Le rétro et streamex
// restent tous au menu : un compte sans la console en voit et télécharge les
// jeux. Remote Play ne fait que jouer : seules ses consoles accordées y sont.
function buildMenuItems(consoles, t, grants) {
	return [
		{
			// One section per service: retrogaming consoles (libretro cores)
			// live here, streamex and remoteplay have their own sections
			// below.
			hidden: true,
			label: t("menu.retrogaming"),
			path: { to: "/settings", component: "gameflex.profile.Settings" },
			children: [],
		},
		{
			// One section per service: retrogaming consoles (libretro cores)
			// live here, streamex and remoteplay have their own sections
			// below.
			hidden: false,
			label: t("menu.retrogaming"),
			path: { to: null, component: null },
			children: [
				...consoles
					.map((c) => {
						// External consoles (switch, ps3 — streamex containers)
						// live in their own "High Definition" section below.
						if (c.method === "remoteplay" || c.method === "streamex") {
							return null;
						}

						return {
							hidden: false,
							icon: "bi bi-controller",
							label: c.name,
							badge: c.year || null,
							path: {
								to: `/retrogaming/${c.id}`,
								component: "gameflex.games.Games",
							},
							children: [],
						};
					})
					.filter(Boolean)
					.sort(byName),
				// Full-screen streaming/watch page — not offcanvas-eligible
				// (video + virtual controller need the whole viewport).
				// Unified across all 3 platforms AND both play/watch — one
				// single route (gameflex/Streaming.jsx), no separate /watch:
				// whether you land as owner/player/spectator is decided
				// server-side from session.me.role (see Streaming.jsx's own
				// doc), never from a query param. Session state itself
				// (platform/consoleId/reference/title) comes entirely from
				// ?sessionId=, looked up via libs/SessionPool.js.
				{
					hidden: true,
					template: false,
					label: t("menu.streaming"),
					path: {
						to: `/streaming`,
						component: "gameflex.Streaming",
					},
					children: [],
				},
				// Full-page keytest preview (Profile → Gamepad → Positioning →
				// "Test") — a real route, not the offcanvas: the stream must
				// survive a reload and the offcanvas's own close fought the
				// Escape blocking of the keytest pad. Spawns/reuses the
				// user's keytest session (KeytestLive.jsx, P2 §5).
				{
					hidden: true,
					template: false,
					label: t("menu.keytest"),
					path: {
						to: `/keytest/:preset`,
						component: "gameflex.profile.settings.gamepad.KeytestLive",
					},
					children: [],
				},
			],
		},
		{
			// Streamex consoles (switch/ps3/xbox) — full HD systems in their
			// own section, same Games.jsx browser as retro-gaming.
			hidden: false,
			label: t("menu.streamex"),
			path: { to: null, component: null },
			children: [
				...consoles
					.map((c) => {
						if (c.method !== "streamex") {
							return null;
						}

						return {
							hidden: false,
							icon: "bi bi-tv",
							label: c.name,
							badge: c.year || null,
							path: {
								to: `/streamex/${c.id}`,
								component: "gameflex.games.Games",
							},
							children: [],
						};
					})
					.filter(Boolean)
					.sort(byName),
			],
		},
		{
			hidden: false,
			label: t("menu.remotePlay"),
			path: { to: null, component: null },
			children: [
				...consoles
					.map((c) => {
						if (c.method !== "remoteplay" || !allows(grants, c)) {
							return null;
						}

						return {
							hidden: false,
							icon: "bi bi-broadcast",
							label: c.name,
							badge: c.year || null,
							path: {
								to: `/remoteplay/${c.id}`,
								component: "gameflex.RemotePlay",
							},
							children: [],
						};
					})
					.filter(Boolean)
					.sort(byName),
			],
		},
		// Une section sans rien dedans ne s'affiche pas.
		//
		// Sur un serveur, chaque section a toujours quelque chose ; sur un
		// téléphone qui n'a que des jeux NES, « STREAMEX » se réduisait à un
		// titre suivi de vide — ce qui ressemble à un chargement en panne bien
		// plus qu'à une absence. Les sections sans enfants (path.to null) sont
		// des en-têtes : les garder vides ne dit rien à personne.
		//
		// Rien de VISIBLE, plus exactement : « Retro gaming » porte des
		// routes cachées (la page de jeu, le test des touches), et un
		// téléphone sans jeu rétro montrait son titre au-dessus du vide
		// (relevé le 2026-10-11).
	].filter((item) => item.path?.to !== null || (item.children || []).some((child) => !child.hidden));
}

// Notification bell — replaces the old fullscreen-only icon in this same
// top-right slot (see TopRightNavigation prop on <Skeleton> below).
// Fullscreen itself is untouched everywhere else it's used (e.g.
// StreamingShell.jsx's own footer button during a game) — this is only
// about the main app chrome's top-right icon. User/profile actions
// (History/SaveStates/Settings/Logout) live in the left
// menu's "Profil" section instead, not here — see buildMenuItems.
// Join/watch invitations received live over the presence WebSocket (see
// PresenceContext.jsx's `invites` — python/src/app/push/routes.py's
// send_invite is what puts them there). Push notifications (closed/
// backgrounded tab) are the OTHER delivery path for the exact same event —
// see service-worker.js — this dropdown only ever shows what arrived while
// this tab was open.
//
// Also lists every currently-active session (useSessionPool — same live WS
// feed the Dashboard's "En direct" carousel and SessionPool.hasOpenSlot use,
// see libs/SessionPool.js) — replaces the old per-console ActiveSessions
// widget that used to sit at the top of Games.jsx (see its own removal doc):
// a session opening anywhere now surfaces here directly instead of only
// being visible to someone already browsing that exact console's page.
function NotificationBell() {
	const { t } = useTranslation();
	const navigate = useNavigate();
	const presence = usePresence();
	const invites = presence?.invites || [];
	const sessions = useSessionPool();
	const handleInviteClick = (invite) => {
		presence.dismissInvite(invite.id);
		navigate(buildStreamingUrl({ sessionId: invite.sessionId }));
	};
	const handleSessionClick = (session) => {
		navigate(buildStreamingUrl({ sessionId: session.id }));
	};
	// LE TÉLÉCHARGEMENT EST UNE NOTIFICATION, pas un bandeau.
	//
	// Il flottait en bas de l'écran, par-dessus la pagination et tout ce qui
	// s'y trouvait. C'est pourtant exactement ce que cette cloche existe pour
	// dire : quelque chose se passe en arrière-plan, va voir si tu veux. Ici
	// il ne gêne personne, et il se trouve à côté des sessions en cours —
	// même famille d'information.
	const download = useDownloadProgress();
	const totalCount = invites.length + sessions.length + (download ? 1 : 0);
	// ANNULER UN TÉLÉCHARGEMENT lancé par erreur — sur l'appareil seulement,
	// le seul hôte qui sache l'arrêter (capacité `cancelDownload`).
	// Confirmé d'abord : un jeu de douze gigaoctets à 97 % ne se relance pas
	// d'un geste.
	const context = useContext(Skeleton.context);
	const cancellable = Boolean(download) && can("cancelDownload");
	const handleCancelDownload = () => {
		context.modal.open(t("notifications.cancelDownloadMessage"), {
			variant: "confirm",
			title: t("notifications.cancelDownloadTitle"),
			confirmLabel: t("notifications.cancelDownload"),
			cancelLabel: t("notifications.keepDownloading"),
			onConfirm: async () => {
				const failure = await deviceCancelDownload();
				if (failure) {
					context.toast.open(t("notifications.cancelDownloadFailed"), { variant: "error" });
				}
			},
		});
	};

	return (
		<>
			{totalCount === 0 ? (
				<h4 className="text-muted text-center py-5">
					<i className="bi bi-bell-slash fs-1"></i>
				</h4>
			) : (
				<>
					{invites.length > 0 && (
						<h5 className="text-uppercase fw-semibold mt-3">
							{t("notifications.categoryInvites")}
						</h5>
					)}
					{invites.map((invite) => (
						<ListElement
							key={`invite-${invite.id}`}
							icon="bi bi-eye-fill"
							onClick={() => handleInviteClick(invite)}
							label={t("notifications.invite", {
								name: invite.from?.displayName || "?",
							})}
							helpText={invite.reference}
						/>
					))}
					{download && (
						<>
							<h5 className="text-uppercase fw-semibold mt-3">
								{t("notifications.categoryDownloads")}
							</h5>
							<ListElement
								key="download"
								icon="bi bi-download"
								onClick={cancellable ? handleCancelDownload : undefined}
								label={download.label || t("notifications.downloading")}
								helpText={download.total > 0 && download.percent >= 0
									? `${formatSize(download.done)} / ${formatSize(download.total)} (${download.percent}%)`
									: t("notifications.downloadedSoFar", { size: formatSize(download.done) })}
								/* L'ANNEAU EST TOUJOURS LÀ, et c'est son
								   `percent` qui change de sens : un nombre le
								   remplit, `null` le fait TOURNER. Quand c'est
								   le serveur qui transfère (un rapatriement
								   depuis un pair), la page ne compte aucun
								   octet — une rotation ne promet rien d'autre
								   que « ça travaille », là où un anneau absent
								   laissait croire que rien ne se passait. */
								next={<DownloadRing
									percent={download.percent >= 0 ? download.percent : null} />}
							/>
						</>
					)}
					{sessions.length > 0 && (
						<h5 className="text-uppercase fw-semibold mt-3">
							{t("notifications.categoryLive")}
						</h5>
					)}
					{sessions.map((session) => (
						<ListElement
							key={`session-${session.id}`}
							icon={hasOpenSlot(session) ? "bi bi-controller" : "bi bi-eye"}
							onClick={() => handleSessionClick(session)}
							label={t("notifications.sessionLive", { title: session.title })}
							helpText={`${session.ownerDisplayName} · ${session.consoleId?.toUpperCase()}`}
						/>
					))}
				</>
			)}
		</>
	);
}


// The actual app shell, gated on authentication — separate from App() below
// since useAuth() needs to run INSIDE <AuthProvider>.
function AppShell() {
	const { t } = useTranslation();
	const { isAuthenticated, isLoading, token, user } = useAuth();
	const consoles = useConsoles();
	const consolesLoaded = useConsolesLoaded();
	const location = useLocation();
	const navigate = useNavigate();

	// Public spectate — watching a running session is protected only by the
	// session id itself (the cast receiver / TV route): no login required,
	// taking control still is. The old check read ?role=spectate, a query
	// param that no longer exists anywhere (role now comes from
	// session.me.role server-side) — which silently gated EVERY anonymous
	// visitor behind the Login page (regression reported 2026-08-23).
	//
	// Le lien d'invitation d'un téléphone (`guest=1&host=…`) compte AUSSI :
	// la règle ne regardait que `sessionId`, si bien qu'un lien partagé
	// depuis une partie du téléphone tombait sur la page de connexion — chez
	// quelqu'un qui n'a aucun compte à y présenter (signalé le 2026-09-02).
	const isPublicSpectate = location.pathname === "/streaming" && (() => {
		const params = new URLSearchParams(location.search);

		return params.has("sessionId") || (params.get("guest") === "1" && params.has("host"));
	})();

	// Every physical gamepad gets a default mapping per virtual controller
	// as soon as it connects (create-only — see gameflex/hooks/use-auto-map-gamepads.js).
	useAutoMapGamepads(isAuthenticated ? token : null);

	// LE ROUTEUR ATTEND LA LISTE QUI FAIT SES ROUTES.
	//
	// Les routes des consoles naissent du menu, et le menu de la liste des
	// consoles, qui arrive après le premier rendu. Recharger /streamex/ps3
	// tombait sur un routeur qui ne connaissait encore aucune console : sa
	// route par défaut renvoyait à l'accueil, et l'adresse était perdue
	// (2026-09-11). Tant qu'elle manque, une adresse inconnue attend au lieu de
	// rediriger. Le menu ne dépend plus des listes de jeux ni des listes
	// fédérées (2026-10-11) : il n'y a plus qu'elle à attendre.
	const routesPending = isAuthenticated && !consolesLoaded;
	const init = useMemo(
		() => ({
			...baseInit,
			routesPending,
			menuItems: buildMenuItems(consoles, t, user?.consoles),
			// UN BANC D'ESSAI SE DIT (GAMEFLEX_STAGING, le porteur, 2026-10-26 :
			// « qu'on ait une différence dans l'interface entre les deux »).
			nameTag: can("staging") ? t("app.staging") : "",
		}),
		[consoles, t, routesPending, user?.consoles],
	);

	// Et l'onglet du navigateur aussi : deux GameFlex ouverts côte à côte se
	// distinguent sans lire la page.
	useEffect(() => {
		document.title = can("staging") ? `${baseInit.name} (${t("app.staging")})` : baseInit.name;
	}, [t]);

	// LE LIEN D'UNE INVITATION (chantier 33) s'adresse à un serveur, pas à
	// cette page : connecté ou non, on dit où le coller.
	if (location.pathname.startsWith("/invite/")) {
		return <InvitePage />;
	}

	if (isLoading) {
		return <LoadingScreen />;
	}

	// Full app gate — no invitation, no access, consistent with the
	// "invitation only, admin/root creates accounts" requirement (see
	// python/src/app/users/routes.py: no /api/auth/register route).
	// The one exception is the spectate route (see isPublicSpectate above):
	// it renders the shell with no account chrome.
	if (!isAuthenticated && !isPublicSpectate) {
		return <Login />;
	}

	const TopRightNavigation = isAuthenticated ? () => {
		const { t } = useTranslation();
		const context = useContext(Skeleton.context);
		// The unread count MUST be computed here, at the mount point, not
		// inside NotificationBell: that component only ever renders as the
		// offcanvas BODY, so a count living there exists only once the panel
		// is already open — which is precisely when a badge is useless. The
		// badge was lost this way when the bell moved from a bs.Dropdown
		// (whose toggle carried it) to an offcanvas opened from a bare <a>.
		// It matters more than it looks: this is how a hand-over request
		// reaches the current pad holder, so no badge means no hand-over.
		const presence = usePresence();
		const liveSessions = useSessionPool();
		// UN TÉLÉCHARGEMENT COMPTE AUSSI. Il manquait ici, alors que le corps
		// de la cloche le comptait déjà dans son propre `totalCount` : un
		// transfert en cours ne marquait donc jamais l'icône, et il fallait
		// ouvrir le panneau pour apprendre qu'il se passait quelque chose —
		// précisément ce que la mise en garde ci-dessus décrit.
		const download = useDownloadProgress();
		const unreadCount = (presence?.invites?.length || 0) + (liveSessions?.length || 0)
			+ (download ? 1 : 0);
		const openNotifications = () => {
			context.offcanvas.open(
				<OffCanvas
					icon="bi bi-bell"
					label={t("notifications.title")}
					helpText={t("profile.notificationsHelp")}
				>
					<NotificationBell />
				</OffCanvas>, {
				size: 600,
			});
		};
		const openSettings = () => navigate("/settings", { replace: true });

		return (
			<div className="d-flex align-items-center gap-3">
				{/* LA CROIX S'Y ARRÊTE (le porteur, 2026-10-21) : un <a> sans
					href ni tabIndex ne prend pas le focus, et la navigation de
					Skeleton ne voyait ni la cloche ni l'engrenage. */}
				<a
					role="button"
					className="position-relative"
					tabIndex={0}
					title={t("notifications.title")}
					aria-label={t("notifications.title")}
					onKeyDown={Skeleton.keyActivate(openNotifications)}
					onClick={openNotifications}
				>
					<i className="bi bi-bell fs-4"></i>
					{unreadCount > 0 && (
						<bs.Badge
							pill
							bg="danger"
							className="position-absolute top-0 start-100 translate-middle"
						>
							{unreadCount}
						</bs.Badge>
					)}
				</a>
				<a
					role="button"
					tabIndex={0}
					title={t("profile.settingsMenu")}
					aria-label={t("profile.settingsMenu")}
					onKeyDown={Skeleton.keyActivate(openSettings)}
					onClick={openSettings}
				>
					<i className="bi bi-gear fs-4"></i>
				</a>
			</div>
		);
	} : null;

	return (
		<PresenceProvider>
			<SessionPoolProvider>
				<Suspense fallback={<LoadingScreen />}>
					<Skeleton init={init} topRightNavigation={TopRightNavigation ? <TopRightNavigation /> : null} />
					{/* La barre flottante des téléchargements : fixée au bas de
						l'écran, visible depuis n'importe quelle page pendant qu'un
						jeu s'importe (2026-08-30). */}
				</Suspense>
			</SessionPoolProvider>
		</PresenceProvider>
	);
}

function App() {
	return (
		<BrowserRouter>
			<ConsolesProvider>
				<AuthProvider>
					<AppShell />
				</AuthProvider>
			</ConsolesProvider>
		</BrowserRouter>
	);
}

// L'hôte déclare ses capacités AVANT le premier rendu : sur le téléphone
// (mobilekernel), /api/v1/capabilities dit « pas de comptes » et l'écran de
// connexion n'existe pas — décider au premier rendu ferait flasher la page
// de login avant la vraie forme. Sur le web sans cet endpoint, le fetch
// échoue vite et rien ne change.
loadHostCapabilities().finally(() => {
	// UNE PAGE SERVIE PAR LE KERNEL N'A RIEN À DÉCLARER.
	//
	// Cette déclaration date de C7-P2, quand la page venait des assets de
	// l'application (`appassets://`) : son origine ne désignait alors aucune
	// API, et il fallait nommer le kernel local. Depuis C7-P3, la WebView
	// charge `http://127.0.0.1:10200/` — le kernel lui-même (MainActivity's
	// PWA_URL). L'origine EST l'API, et la déclaration ne sert plus à rien
	// sur le téléphone.
	//
	// Elle NUISAIT, en revanche, à tous les autres. Depuis un ordinateur du
	// réseau, la page arrive de `http://192.168.1.14:10200` : l'origine
	// déclarée ne correspond plus, `serverOrigin()` la rend, et chaque appel
	// part vers le 127.0.0.1 DE CETTE MACHINE-LÀ. Le navigateur refuse la
	// connexion, et l'écran de connexion répond « Backend injoignable » à
	// quelqu'un dont le réseau va parfaitement bien (signalé le 2026-09-04).
	//
	// On ne déclare donc que si l'origine de la page ne peut pas adresser
	// l'API — un schéma qui n'est pas http(s). Servie par HTTP, la page
	// parle à celui qui la sert, et c'est toujours le bon.
	const servedOverHttp = window.location.protocol === "http:"
		|| window.location.protocol === "https:";
	if (can("webrtcGateway") && !servedOverHttp) {
		window.GAMEFLEX_SERVER = "http://127.0.0.1:10200";
	}

	// L'ORIENTATION CHOISIE DANS RÉGLAGES (#406). L'application la garde
	// elle-même ; la page la lui redit à chaque ouverture, pour que les deux
	// ne puissent pas se contredire — la page fait foi. Hors de
	// l'application, rien n'est demandé (voir libs/screenOrientation.js).
	applyScreenOrientation();

	// La bibliothèque du téléphone arrive du kernel (HTTP) — chargée avant
	// le premier rendu, comme les capacités.
	preloadLocalLibrary();

	createRoot(document.getElementById("root")).render(<App />);
});

// PWA service worker — production only (restored 2026-07-29: removing it
// entirely did NOT fix the HMR infinite-reload loop reported in dev, so the
// service worker was never the actual/only cause — see the fresh
// investigation this same day, in GAMEFLEX_JOURNAL.md). Registering
// only in production, with a self-heal unregister in dev for any machine
// that already has one installed, is still correct regardless of what the
// real HMR cause turns out to be.
if (process.env.NODE_ENV === "production" && "serviceWorker" in navigator) {
	window.addEventListener("load", () => {
		navigator.serviceWorker
			.register("/service-worker.js")
			.then((reg) => console.log("[PWA] Service Worker registered:", reg.scope))
			.catch((err) =>
				console.log("[PWA] Service Worker registration failed:", err),
			);
	});
} else if ("serviceWorker" in navigator) {
	navigator.serviceWorker.getRegistrations().then((registrations) => {
		registrations.forEach((registration) => registration.unregister());
	});
}
