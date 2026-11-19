#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/../.."
# Use an existing local esbuild binary, or the development dependency installed by the owner.
: "${ESBUILD:=esbuild}"
"$ESBUILD" output/preview-entry.jsx --bundle --outfile=output/experience-preview.js --minify --alias:react="$(pwd)/src/webapp/src/node_modules/react" --alias:react-dom="$(pwd)/src/webapp/src/node_modules/react-dom" '--define:process.env.NODE_ENV="production"'
printf '%s\n' 'Aperçu construit : output/experience-preview.html'
