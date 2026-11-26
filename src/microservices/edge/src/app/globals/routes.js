// LA TABLE DES ROUTES DES RÉGLAGES (chantier 81, §2.2 ; le porteur, 2026-11-24 : « une route doit avoir un fichier jsx
// attribué ») : chaque route, son fichier, au chemin de la route — `/settings/logs/settings` → `settings/logs/Settings.jsx`.
// UN SEGMENT VARIABLE NE FAIT PAS DE DOSSIER (le porteur, 2026-11-26) : les segments fixes sont les dossiers, le dernier
// le nom du fichier — `/settings/profile/saves/:source/console/:consoleId/game/:gameId/note` →
// `settings/profile/saves/console/game/Note.jsx` ; une route qui finit par une variable prend son nom
// (`/settings/consoles/:consoleId` → `settings/consoles/Console.jsx`). Deux routes qui montrent la même page ont chacune
// leur fichier ; le second réexporte le premier.
// `entry` : l'entrée de la liste des Réglages dont elle dépend (son accès : administrateur, capacité de l'hôte — c'est la
// liste qui le sait, Settings.jsx) ; `capability` : une capacité de plus qu'exige la route seule.
//
// UNE ZONE PAS ENCORE REFAITE garde une seule route, son dossier entier (« consoles/* »), vers le fichier qui déclare
// encore ses propres pages : elle passe à un fichier par route quand le chantier y arrive. Une seule table dans les deux
// cas — jamais deux mécanismes.
import Consoles from "../settings/Consoles.jsx";
import ConsolePage from "../settings/consoles/Console.jsx";
import ConsoleStatus from "../settings/consoles/Status.jsx";
import ConsoleParams from "../settings/consoles/params/Emulator.jsx";
import ConsoleCores from "../settings/consoles/Cores.jsx";
import ConsoleMobileCores from "../settings/consoles/MobileCores.jsx";
import ConsoleBios from "../settings/consoles/Bios.jsx";
import ConsoleCore from "../settings/consoles/cores/Core.jsx";
import ConsoleBiosFile from "../settings/consoles/bios/File.jsx";
import ConsoleAppearance from "../settings/consoles/Appearance.jsx";
import ConsoleSlot from "../settings/consoles/appearance/Slot.jsx";
import ConsoleSlotSearch from "../settings/consoles/appearance/Search.jsx";
import ConsoleOrphans from "../settings/consoles/MetadataOrphans.jsx";
import ConsoleGames from "../settings/consoles/Games.jsx";
import GamesSource from "../settings/consoles/games/Source.jsx";
import GamesGame from "../settings/consoles/games/Game.jsx";
import GamesPatch from "../settings/consoles/games/Patch.jsx";
import GamesAppearance from "../settings/consoles/games/Appearance.jsx";
import GamesImage from "../settings/consoles/games/Image.jsx";
import GamesImageSearch from "../settings/consoles/games/ImageSearch.jsx";
import GamesInfos from "../settings/consoles/games/Infos.jsx";
import GamesMatch from "../settings/consoles/games/Match.jsx";
import GamesForget from "../settings/consoles/games/Forget.jsx";
import Credits from "../settings/Credits.jsx";
import { DownloadPage } from "../settings/Download.jsx";
import { LicencePage } from "../settings/Licence.jsx";
import { ProjectPage } from "../settings/Gameflex.jsx";
import { ApplicationPage } from "../settings/Application.jsx";
import Gamepads from "../settings/Gamepads.jsx";
import { LayoutPage as GamepadLayout } from "../settings/gamepads/layout/Controller.jsx";
import { InvitePage } from "../settings/Invite.jsx";
import Catalog from "../settings/catalog/Games.jsx";
import CatalogGame from "../settings/catalog/games/Game.jsx";
import Federation from "../settings/Federation.jsx";
import FederationAccesses from "../settings/federation/Access.jsx";
import FederationAdd from "../settings/federation/Add.jsx";
import FederationInvitation from "../settings/federation/invitations/Invitation.jsx";
import FederationNewPairing from "../settings/federation/NewPairing.jsx";
import FederationNewToken from "../settings/federation/NewToken.jsx";
import FederationPending from "../settings/federation/Pending.jsx";
import FederationServer from "../settings/federation/servers/Server.jsx";
import FederationServers from "../settings/federation/Servers.jsx";
import FederationToken from "../settings/federation/tokens/Token.jsx";
import FederationTokenConsoles from "../settings/federation/tokens/Consoles.jsx";
import Logout from "../settings/Logout.jsx";
import Logs from "../settings/Logs.jsx";
import ServerConfig from "../settings/ServerConfig.jsx";
import Members from "../settings/Members.jsx";
import MemberConsoles from "../settings/members/Consoles.jsx";
import Member from "../settings/members/Member.jsx";
import NewMember from "../settings/members/New.jsx";
import MemberSaves from "../settings/members/Saves.jsx";
import Profile from "../settings/Profile.jsx";
import Favorites from "../settings/profile/Favorites.jsx";
import FavoriteGame from "../settings/profile/favorites/Game.jsx";
import MyFederation from "../settings/profile/Federation.jsx";
import Connections from "../settings/profile/federation/Connections.jsx";
import Connection from "../settings/profile/federation/connections/Connection.jsx";
import Pairing from "../settings/profile/federation/Pairing.jsx";
import Servers from "../settings/profile/federation/Servers.jsx";
import Server from "../settings/profile/federation/servers/Server.jsx";
import Token from "../settings/profile/federation/Token.jsx";
import Saves from "../settings/profile/Saves.jsx";
import SaveConsole from "../settings/profile/saves/console/Console.jsx";
import SaveConsoleGames from "../settings/profile/saves/console/All.jsx";
import SaveConsoleGame from "../settings/profile/saves/console/game/Game.jsx";
import SaveConsoleNote from "../settings/profile/saves/console/game/Note.jsx";
import SaveGame from "../settings/profile/saves/game/Game.jsx";
import SaveNote from "../settings/profile/saves/game/slot/Note.jsx";
import SaveSlots from "../settings/profile/saves/game/All.jsx";
import SaveSlot from "../settings/profile/saves/game/slot/Slot.jsx";
import SaveSource from "../settings/profile/saves/Source.jsx";
import Account from "../settings/profile/Account.jsx";
import Mfa from "../settings/profile/Mfa.jsx";
import Password from "../settings/profile/Password.jsx";
import Log from "../settings/logs/view/Log.jsx";
import LogsSettings from "../settings/logs/Settings.jsx";
import Resources from "../settings/Resources.jsx";
import ResourceAdd from "../settings/resources/Add.jsx";
import ResourceAddKind from "../settings/resources/add/Kind.jsx";
import ResourceDisk from "../settings/resources/disks/Disk.jsx";
import ResourceDisks from "../settings/resources/Disks.jsx";
import ResourceExternals from "../settings/resources/External.jsx";
import ResourceExternal from "../settings/resources/external/External.jsx";
import ResourceExternalEdit from "../settings/resources/external/Edit.jsx";
import ResourceExternalGames from "../settings/resources/external/Games.jsx";
import Cache from "../settings/Cache.jsx";
import CacheEntry from "../settings/cache/Entry.jsx";
import Cast from "../settings/Cast.jsx";
import CastApplication from "../settings/cast/Application.jsx";
import Metadata from "../settings/Metadata.jsx";
import MetadataService from "../settings/metadata/Service.jsx";
import RemotePlay from "../settings/RemotePlay.jsx";
import RemotePlayBackground from "../settings/remote-play/Background.jsx";
import RemotePlayDevice from "../settings/remote-play/Device.jsx";
import RemotePlayNew from "../settings/remote-play/New.jsx";
import { RescanPage } from "../settings/Rescan.jsx";
import { TemplatesRoutes } from "../settings/templates/Templates.jsx";

export const SETTINGS_ROUTES = [
    { entry: "profile", path: "profile", page: Profile },
    { entry: "profile", path: "profile/account", page: Account },
    { entry: "profile", path: "profile/password", page: Password },
    { entry: "profile", path: "profile/mfa", page: Mfa },
    { entry: "profile", path: "profile/federation", page: MyFederation },
    { entry: "profile", path: "profile/federation/token", page: Token },
    { entry: "profile", path: "profile/federation/pairing", page: Pairing },
    { entry: "profile", path: "profile/federation/connections", page: Connections },
    { entry: "profile", path: "profile/federation/connections/:key", page: Connection },
    { entry: "profile", path: "profile/federation/servers", page: Servers },
    { entry: "profile", path: "profile/federation/servers/:serverId", page: Server },
    { entry: "profile", path: "profile/saves", page: Saves },
    { entry: "profile", path: "profile/saves/:source", page: SaveSource },
    { entry: "profile", path: "profile/saves/:source/game/:consoleId/:reference", page: SaveGame },
    { entry: "profile", path: "profile/saves/:source/game/:consoleId/:reference/all", page: SaveSlots },
    { entry: "profile", path: "profile/saves/:source/game/:consoleId/:reference/slot/:slot", page: SaveSlot },
    { entry: "profile", path: "profile/saves/:source/game/:consoleId/:reference/slot/:slot/note", page: SaveNote },
    { entry: "profile", path: "profile/saves/:source/console/:consoleId", page: SaveConsole },
    { entry: "profile", path: "profile/saves/:source/console/:consoleId/all", page: SaveConsoleGames },
    { entry: "profile", path: "profile/saves/:source/console/:consoleId/game/:gameId", page: SaveConsoleGame },
    { entry: "profile", path: "profile/saves/:source/console/:consoleId/game/:gameId/note", page: SaveConsoleNote },
    { entry: "profile", path: "profile/favorites", page: Favorites },
    { entry: "profile", path: "profile/favorites/game", page: FavoriteGame },
    { entry: "catalogGames", path: "catalog/games", page: Catalog },
    { entry: "catalogGames", path: "catalog/games/game", page: CatalogGame },
    { entry: "application", path: "application", page: ApplicationPage },
    { entry: "gamepads", path: "gamepads", page: Gamepads },
    { entry: "gamepads", path: "gamepads/layout/:controller", page: GamepadLayout },
    { entry: "invite", path: "invite", page: InvitePage },
    { entry: "remotePlay", path: "remote-play", page: RemotePlay },
    { entry: "remotePlay", path: "remote-play/new", page: RemotePlayNew },
    { entry: "remotePlay", path: "remote-play/:deviceId", page: RemotePlayDevice },
    { entry: "remotePlay", path: "remote-play/:deviceId/background", page: RemotePlayBackground },
    { entry: "consoles", path: "consoles", page: Consoles },
    { entry: "consoles", path: "consoles/:consoleId", page: ConsolePage },
    { entry: "consoles", path: "consoles/:consoleId/status", page: ConsoleStatus },
    { entry: "consoles", path: "consoles/:consoleId/params/:emulator", page: ConsoleParams },
    { entry: "consoles", path: "consoles/:consoleId/cores", page: ConsoleCores },
    { entry: "consoles", path: "consoles/:consoleId/mobile-cores", page: ConsoleMobileCores },
    { entry: "consoles", path: "consoles/:consoleId/bios", page: ConsoleBios },
    { entry: "consoles", path: "consoles/:consoleId/cores/:name", page: ConsoleCore },
    { entry: "consoles", path: "consoles/:consoleId/bios/:name", page: ConsoleBiosFile },
    { entry: "consoles", path: "consoles/:consoleId/appearance", page: ConsoleAppearance },
    { entry: "consoles", path: "consoles/:consoleId/appearance/search", page: ConsoleSlotSearch },
    { entry: "consoles", path: "consoles/:consoleId/appearance/:slot", page: ConsoleSlot },
    { entry: "consoles", path: "consoles/:consoleId/metadata-orphans", page: ConsoleOrphans },
    { entry: "consoles", path: "consoles/:consoleId/games", page: ConsoleGames },
    { entry: "consoles", path: "consoles/:consoleId/games/:source", page: GamesSource },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/game", page: GamesGame },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/patch", page: GamesPatch },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/appearance", page: GamesAppearance },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/image", page: GamesImage },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/image-search", page: GamesImageSearch },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/infos", page: GamesInfos },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/match", page: GamesMatch },
    { entry: "consoles", path: "consoles/:consoleId/games/:source/forget", page: GamesForget },
    { entry: "cache", path: "cache", page: Cache },
    { entry: "cache", path: "cache/:consoleId/:name", page: CacheEntry },
    { entry: "serverConfig", path: "server-config", page: ServerConfig, capability: "serverConfig" },
    { entry: "logs", path: "logs", page: Logs },
    { entry: "logs", path: "logs/settings", page: LogsSettings, capability: "logSettings" },
    { entry: "logs", path: "logs/view/*", page: Log },
    { entry: "members", path: "members", page: Members },
    { entry: "members", path: "members/new", page: NewMember },
    { entry: "members", path: "members/:memberId", page: Member },
    { entry: "members", path: "members/:memberId/consoles", page: MemberConsoles },
    { entry: "members", path: "members/:memberId/saves", page: MemberSaves },
    { entry: "resources", path: "resources", page: Resources },
    { entry: "resources", path: "resources/disks", page: ResourceDisks },
    { entry: "resources", path: "resources/disks/:name", page: ResourceDisk },
    { entry: "resources", path: "resources/external", page: ResourceExternals },
    { entry: "resources", path: "resources/external/:id", page: ResourceExternal },
    { entry: "resources", path: "resources/external/:id/games", page: ResourceExternalGames },
    { entry: "resources", path: "resources/external/:id/edit", page: ResourceExternalEdit },
    { entry: "resources", path: "resources/add", page: ResourceAdd },
    { entry: "resources", path: "resources/add/:kind", page: ResourceAddKind },
    { entry: "federation", path: "federation", page: Federation },
    { entry: "federation", path: "federation/access", page: FederationAccesses },
    { entry: "federation", path: "federation/pending", page: FederationPending },
    { entry: "federation", path: "federation/new-token", page: FederationNewToken },
    { entry: "federation", path: "federation/new-pairing", page: FederationNewPairing },
    { entry: "federation", path: "federation/add", page: FederationAdd },
    { entry: "federation", path: "federation/servers", page: FederationServers },
    { entry: "federation", path: "federation/servers/:serverId", page: FederationServer },
    { entry: "federation", path: "federation/invitations/:invitationId", page: FederationInvitation },
    { entry: "federation", path: "federation/tokens/:tokenId", page: FederationToken },
    { entry: "federation", path: "federation/tokens/:tokenId/consoles", page: FederationTokenConsoles },
    { entry: "metadata", path: "metadata", page: Metadata },
    { entry: "metadata", path: "metadata/:service", page: MetadataService },
    { entry: "cast", path: "cast", page: Cast },
    { entry: "cast", path: "cast/application", page: CastApplication },
    { entry: "rescan", path: "rescan", page: RescanPage },
    { entry: "gameflex", path: "gameflex", page: ProjectPage },
    { entry: "licence", path: "licence", page: LicencePage },
    { entry: "credits", path: "credits", page: Credits },
    { entry: "download", path: "download", page: DownloadPage },
    { entry: "logout", path: "logout", page: Logout },
    { entry: "templates", path: "templates/*", page: TemplatesRoutes },
];

// L'adresse d'une entrée : sa première route.
export const settingsPath = (entry) => `/settings/${SETTINGS_ROUTES.find((route) => route.entry === entry).path.replace("/*", "")}`;
