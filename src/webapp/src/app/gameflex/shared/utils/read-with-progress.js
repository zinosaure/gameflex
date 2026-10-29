/**
 * Le corps d'une réponse, lu par morceaux, en annonçant l'avancement.
 *
 * `response.blob()` ne rend la main qu'à la fin : un ROM de plusieurs
 * gigaoctets laissait donc un bouton grisé et muet pendant des minutes, ce
 * qui se lit comme une panne. Le lecteur, lui, dit où il en est.
 *
 * Sans taille connue, il n'y a pas de pourcentage à calculer : on rend le
 * blob sans prétendre en connaître un — l'appelant affiche alors une attente,
 * pas une barre qui mentirait. Deux cas où elle manque : un jeu multi-fichiers
 * que le serveur zippe à la volée (il ne sait pas d'avance ce que ça pèsera),
 * et une réponse simplement diffusée en morceaux.
 *
 * `expected` est là pour ce second cas : l'appelant connaît souvent la taille
 * par ailleurs — un listing la porte — et ce serait dommage de tourner dans
 * le vide quand on sait compter.
 */
export async function readWithProgress(response, onProgress, expected = 0) {
    const total = Number(response.headers.get("Content-Length")) || expected || 0;
    if (!total || !response.body?.getReader) {
        return response.blob();
    }

    const reader = response.body.getReader();
    const chunks = [];
    let done = 0;
    let lastPercent = -1;

    for (; ;) {
        const step = await reader.read();
        if (step.done) {
            break;
        }

        chunks.push(step.value);
        done += step.value.length;
        const percent = Math.floor((done * 100) / total);
        if (percent !== lastPercent) {
            lastPercent = percent;
            onProgress({ percent, done, total });
        }
    }

    return new Blob(chunks, { type: response.headers.get("Content-Type") || "application/octet-stream" });
}

