// Le pont entre le vocabulaire du DESSIN (`CROSS`, `DPAD_UP`) et les
// identifiants du PROFIL de la console (`cross`, `dup`, servis par
// /profile.json).
//
// Ce fichier existe parce que la même fonction était recopiée dans les QUATRE
// pads streamex — à l'octet près, vérifié par empreinte. Une correction sur
// l'une des copies (il y en a eu : les deux replis ci-dessous datent du
// 2026-09-07) ne se propageait pas aux trois autres.

// resolveProfileButton — trouve l'entrée de profil qui correspond à un nom du
// gabarit.
//
// La table de traduction reste utile : elle relie un vocabulaire d'affichage
// (`DPAD_UP`) à un identifiant qu'aucune règle simple ne dérive (`dup`). Mais
// on ne s'y arrête plus : à défaut, le nom est essayé tel quel, puis en
// minuscules. Trois chances de trouver au lieu d'une, sur des vocabulaires qui
// ont divergé trois fois dans la même journée.
export function resolveProfileButton(profileButtons, mapping, name) {
	const candidates = [mapping[name], name, String(name).toLowerCase()];

	for (const candidate of candidates) {
		if (!candidate) {
			continue;
		}

		const found = profileButtons.find((entry) => entry.id === candidate);
		if (found) {
			return found;
		}
	}

	return null;
}

// makeCanDrawButton — le prédicat que ControllerLayout attend.
//
// Un bouton du gabarit qui ne correspond à AUCUNE entrée du profil n'a pas de
// destination : le dessiner donne un bouton qui s'allume sous le doigt et
// n'envoie rien (le TOUCHPAD du gabarit `dualsense`, que ni ps4.json ni
// ps5.json ne déclarent). Mieux vaut ne pas le dessiner.
//
// Les sticks font exception : ils ne passent pas par les boutons du profil
// mais par `onAnalogMove`/`st,…`, donc leur nom est absent du profil sans que
// ça veuille dire quoi que ce soit. Ils sont nommés par l'appelant, qui est le
// seul à savoir lesquels il gère.
export function makeCanDrawButton(profileButtons, mapping, analogNames = []) {
	// PROFIL PAS ENCORE LÀ = ON DESSINE TOUT.
	//
	// /profile.json arrive après le premier rendu. Sans ce garde-fou, le pad
	// apparaîtrait VIDE le temps de l'aller-retour — et resterait vide si la
	// requête échouait. Un bouton muet est un défaut ; un pad absent en est un
	// pire.
	if (!profileButtons || profileButtons.length === 0) {
		return () => true;
	}

	const analog = new Set(analogNames);

	return (name) =>
		analog.has(name) || resolveProfileButton(profileButtons, mapping, name) !== null;
}
