import icons from './console-icons.json';
import {appPath} from '../libs/appBase.js';

// Exact identifiers only: never confuse Steam with Steam Deck, CPC with GX4000,
// or the Capcom arcade boards with Neo Geo. Uncovered machines retain their art.
export const consoleIconAliases = {
    nes:'nintendo-nes', snes:'super-nintendo', gb:'game-boy', gbc:'game-boy-color',
    gba:'game-boy-advance', vb:'virtual-boy', n64:'nintendo-64', nds:'nintendo-ds',
    gc:'nintendo-gamecube', gamecube:'nintendo-gamecube', wii:'nintendo-wii',
    sms:'sega-master-system', megadrive:'sega-mega-drive', genesis:'sega-mega-drive',
    saturn:'sega-saturn', dreamcast:'sega-dreamcast', gamegear:'sega-game-gear',
    sg1000:'sega-sg-1000', segacd:'sega-mega-cd', sega32x:'sega-32x',
    atari2600:'atari-2600', atari5200:'atari-5200', atari7800:'atari-7800',
    lynx:'atari-lynx', jaguar:'atari-jaguar', pce:'pc-engine', pcfx:'nec-pc-fx',
    neogeo:'neo-geo-aes', ngpc:'neo-geo-pocket-color', wonderswancolor:'wonderswan-color',
    coleco:'colecovision', ps1:'playstation-1', psx:'playstation-1',
    ps2:'playstation-2', psp:'playstation-portable', psvita:'playstation-vita',
    ps3:'playstation-3', ps4:'playstation-4', ps5:'playstation-5',
    wiiu:'wii-u', switch:'nintendo-switch', switch2:'nintendo-switch-2',
    xbox360:'xbox-360', xboxone:'xbox-one', xboxseries:'xbox-series-x',
    'rp-ps4':'playstation-4', 'rp-ps5':'playstation-5',
    'rp-xboxseries':'xbox-series-x', 'rp-switch2':'nintendo-switch-2',
};

export function consoleIcon(consoleId) {
    const slug = Object.prototype.hasOwnProperty.call(consoleIconAliases,consoleId) ? consoleIconAliases[consoleId] : consoleId;
    const icon = Object.prototype.hasOwnProperty.call(icons,slug) ? icons[slug] : null;
    return icon ? {...icon, slug, src:appPath(`/images/console-icons/${icon.file}`)} : null;
}
