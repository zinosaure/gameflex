#!/usr/bin/env bash
# GameFlex: set up and run your server.
#
#   ./launch.sh                  the first time: a few questions, then GameFlex starts
#                                afterwards: starts GameFlex
#   ./launch.sh setup            answer the questions again (rewrites .env), then start
#   ./launch.sh consoles ps3 switch
#                                download recent consoles (ps3 ps4 ps5 switch wiiu xbox xbox360)
#   ./launch.sh wireguard        turn on the optional WireGuard VPN, to play away from home
#                                without putting GameFlex on the Internet
#   ./launch.sh update           download the latest version, with its docker-compose.yml
#                                (your disks kept) and launch.sh, then restart
#   ./launch.sh stop             stop GameFlex (your library and settings stay)
#   ./launch.sh status           show what is running
#   ./launch.sh logs [service]   follow the log of a service: edge, kernel, credential,
#                                gateway, remoteplay or wireguard (the gameflex- prefix is optional)
#   ./launch.sh reset-password   set the administrator password back to the one in .env
#                                (it only sets it at the first start; use this if you lost it)
#
# Every answer can also come from the environment, for an install without
# questions: GAMEFLEX_LIBRARY, GAMEFLEX_PUBLIC_HOST, GAMEFLEX_ROOT_PASSWORD,
# COMPOSE_PROFILES.
set -euo pipefail
cd "$(dirname "$0")"

RAW=https://raw.githubusercontent.com/zinosaure/gameflex/main
CONSOLES="ps3 ps4 ps5 switch wiiu xbox xbox360"
# The optional services, turned on the same way as the consoles (COMPOSE_PROFILES).
EXTRAS="wireguard"

say() { printf '%s\n' "$*"; }
fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }

# ── What this computer needs ────────────────────────────────────────

check_requirements() {
    command -v docker >/dev/null 2>&1 ||
        fail "Docker is not installed. See https://docs.docker.com/engine/install/"
    docker compose version >/dev/null 2>&1 ||
        fail "Docker Compose v2 is missing (the 'docker compose' command)."
    docker info >/dev/null 2>&1 ||
        fail "Docker does not answer. Start it, or add yourself to the 'docker' group (then sign out and in again)."
    [ -e /dev/dri ] ||
        say "Warning: no /dev/dri on this computer. GameFlex needs a graphics card to encode the video."
    [ -e /dev/uinput ] ||
        say "Warning: no /dev/uinput. Recent consoles will have no controller. Fix it with: sudo modprobe uinput"
    return 0
}

need_file() {
    [ -f "$1" ] && return 0
    say "Downloading $1…"
    curl -fsSL -o "$1" "$RAW/$1" || fail "could not download $1 from $RAW"
}

# ── Updating the files ──────────────────────────────────────────────

# files_url — where the files of the version in .env are: its tag when
# GAMEFLEX_VERSION names one (0.4.0), the newest otherwise (latest).
files_url() {
    local version
    version=$(get_setting GAMEFLEX_VERSION)
    case "$version" in
        [0-9]*.[0-9]*.[0-9]*) printf '%s\n' "${RAW%/main}/v$version" ;;
        *) printf '%s\n' "$RAW" ;;
    esac
}

# carry_disks OLD NEW — prints NEW with the disks added to OLD: every
# `<folder>:/mnt/<name>` line, with the comments just above it, goes after
# the example line `# - /media/games:/mnt/games`. Fails when NEW has no such
# line to put them after.
carry_disks() {
    awk -v example='# - /media/games:/mnt/games' '
        FNR == 1 { file++ }
        file == 1 { seen[$0] = 1; next }
        file == 2 {
            if ($0 ~ /^[ \t]*#/) {
                if (!($0 in seen)) comments = comments $0 "\n"
                next
            }
            if ($0 ~ /^[ \t]*-[ \t]*[^# \t][^#]*:\/mnt\// && !($0 in seen)) disks = disks comments $0 "\n"
            comments = ""
            next
        }
        { print }
        !done && index($0, example) { printf "%s", disks; done = 1 }
        END { if (disks != "" && !done) exit 1 }
    ' "$2" "$1" "$2"
}

# update_files — docker-compose.yml, launch.sh and .env.example of the version
# to run. The disks you added move into the new compose file, and the previous
# one stays in docker-compose.yml.old.
update_files() {
    local base temporary self
    base=$(files_url)
    self=$(basename "$0")
    # Next to the files, so that moving one in place is a rename: bash is
    # still reading this script, and keeps reading the old one.
    temporary=$(mktemp -d ./.update.XXXXXX)
    if ! curl -fsSL -o "$temporary/docker-compose.yml" "$base/docker-compose.yml" ||
        ! curl -fsSL -o "$temporary/launch.sh" "$base/launch.sh" ||
        ! curl -fsSL -o "$temporary/.env.example" "$base/.env.example"; then
        rm -rf "$temporary"
        say "Warning: could not download the files of this version from $base; yours stay as they are."
        return 0
    fi
    mv "$temporary/.env.example" .env.example

    if carry_disks docker-compose.yml "$temporary/docker-compose.yml" > "$temporary/merged" &&
        docker compose -f "$temporary/merged" --project-directory . config --quiet 2>/dev/null; then
        if ! cmp -s "$temporary/merged" docker-compose.yml; then
            cp docker-compose.yml docker-compose.yml.old
            mv "$temporary/merged" docker-compose.yml
            say "docker-compose.yml updated, with your disks; the previous one is in docker-compose.yml.old."
        fi
    else
        mv "$temporary/docker-compose.yml" docker-compose.yml.new
        say "Warning: docker-compose.yml could not be updated by itself. The new one is"
        say "docker-compose.yml.new: copy your changes into it, then rename it docker-compose.yml."
    fi

    if ! cmp -s "$temporary/launch.sh" "$self"; then
        chmod +x "$temporary/launch.sh"
        mv "$temporary/launch.sh" "$self"
        say "launch.sh updated."
    fi
    rm -rf "$temporary"
}

# ── .env ────────────────────────────────────────────────────────────

# set_setting KEY VALUE — writes KEY=VALUE in .env, replacing the line if it is
# there. Done in bash rather than sed: a password may hold any character.
set_setting() {
    local key=$1 value=$2 line found=0 temporary
    temporary=$(mktemp)
    while IFS= read -r line || [ -n "$line" ]; do
        if [ "${line%%=*}" = "$key" ]; then
            printf '%s=%s\n' "$key" "$value"
            found=1
        else
            printf '%s\n' "$line"
        fi
    done < .env > "$temporary"
    [ "$found" = 1 ] || printf '%s=%s\n' "$key" "$value" >> "$temporary"
    mv "$temporary" .env
    chmod 600 .env
}

get_setting() {
    sed -n "s/^$1=//p" .env 2>/dev/null | tail -n 1
}

ask() {
    local question=$1 default=$2 answer
    if [ ! -t 0 ]; then
        printf '%s\n' "$default"
        return
    fi
    read -r -p "$question [$default]: " answer
    printf '%s\n' "${answer:-$default}"
}

detect_address() {
    ip -4 route get 1.1.1.1 2>/dev/null |
        awk '{for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit }}'
}

make_secret() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
    else
        head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
    fi
}

setup() {
    need_file docker-compose.yml
    need_file .env.example
    [ -f .env ] || cp .env.example .env
    chmod 600 .env

    say ""
    say "GameFlex setup. Press Enter to keep the value in brackets."
    say ""

    local library address password confirm profiles
    library=${GAMEFLEX_LIBRARY:-$(ask "Library folder (your games and saves)" "$(pwd)/library")}
    case "$library" in
        /*) ;;
        *) library="$(pwd)/$library" ;;
    esac
    mkdir -p "$library" || fail "could not create $library"

    address=${GAMEFLEX_PUBLIC_HOST:-$(ask "Address of this computer on your network" "$(detect_address)")}
    [ -n "$address" ] || fail "no address: set GAMEFLEX_PUBLIC_HOST, such as 192.168.1.50"

    password=${GAMEFLEX_ROOT_PASSWORD:-}
    if [ -z "$password" ] && [ -t 0 ]; then
        while true; do
            read -r -s -p "Administrator password (user: root; empty to generate one): " password
            say ""
            [ -z "$password" ] && break
            case "$password" in
                *"'"*) say "Please choose a password without a single quote (')."; continue ;;
            esac
            read -r -s -p "Same password again: " confirm
            say ""
            [ "$password" = "$confirm" ] && break
            say "The two passwords differ. Try again."
        done
    fi
    if [ -z "$password" ]; then
        password=$(make_secret | cut -c1-20)
        say "Administrator password generated: $password"
        say "Write it down: you sign in as root with it."
    fi

    profiles=${COMPOSE_PROFILES-$(ask "Recent consoles to download ($CONSOLES), separated by spaces" \
        "$(get_setting COMPOSE_PROFILES | tr ',' ' ')")}

    set_setting GAMEFLEX_LIBRARY "$library"
    set_setting GAMEFLEX_PUBLIC_HOST "$address"
    # Between single quotes: Compose then reads it as is, whatever it holds.
    case "$password" in
        *"'"*) fail "the administrator password must not hold a single quote (')" ;;
    esac
    set_setting GAMEFLEX_ROOT_PASSWORD "'$password'"
    [ -n "$(get_setting GAMEFLEX_SECRET)" ] || set_setting GAMEFLEX_SECRET "$(make_secret)"
    # shellcheck disable=SC2086 # the answer is a list: split it on spaces.
    set_setting COMPOSE_PROFILES "$(profiles_list $profiles)"

    # From now on .env decides: Compose would otherwise prefer these variables,
    # as they came, over what was just written (a relative folder, for one).
    unset GAMEFLEX_LIBRARY GAMEFLEX_PUBLIC_HOST GAMEFLEX_ROOT_PASSWORD COMPOSE_PROFILES

    docker compose config --quiet || fail "the settings in .env are not valid"
    say ""
    say "Settings saved in .env."
}

# profiles_list ps3 switch… — the valid names, comma-separated, as Compose wants them.
profiles_list() {
    local argument name list=""
    for argument in "$@"; do
        for name in ${argument//,/ }; do
            case ",$list," in
                *",$name,"*) continue ;;
            esac
            case " $CONSOLES $EXTRAS " in
                *" $name "*) list="${list:+$list,}$name" ;;
                *) say "Unknown name '$name' (known: $CONSOLES $EXTRAS)." >&2 ;;
            esac
        done
    done
    printf '%s\n' "$list"
}

# ── Running ─────────────────────────────────────────────────────────

start() {
    # Also when .env was written beforehand, by hand or by an agent.
    need_file docker-compose.yml
    say "Downloading GameFlex (the first time takes a few minutes)…"
    docker compose pull
    docker compose up -d
    local port address user
    port=$(get_setting GAMEFLEX_PORT)
    port=${port:-10200}
    address=$(get_setting GAMEFLEX_PUBLIC_HOST)
    user=$(get_setting GAMEFLEX_ROOT_USERNAME)
    user=${user:-root}
    for _ in $(seq 1 60); do
        curl -fs -o /dev/null "http://127.0.0.1:$port/" && break
        sleep 2
    done
    say ""
    say "GameFlex is running. Open http://$address:$port and sign in as $user."
    say "Next: put your games in $(get_setting GAMEFLEX_LIBRARY)/roms/<console>/ (see docs/library.md)."
}

case "${1:-}" in
    "")
        check_requirements
        [ -f .env ] && [ -n "$(get_setting GAMEFLEX_SECRET)" ] || setup
        start
        ;;
    setup)
        check_requirements
        setup
        start
        ;;
    consoles)
        shift
        [ -f .env ] || fail "run ./launch.sh first"
        set_setting COMPOSE_PROFILES "$(profiles_list "$(get_setting COMPOSE_PROFILES)" "$@")"
        docker compose pull
        say "Consoles ready: $(get_setting COMPOSE_PROFILES)"
        ;;
    wireguard)
        [ -f .env ] || fail "run ./launch.sh first"
        set_setting COMPOSE_PROFILES "$(profiles_list "$(get_setting COMPOSE_PROFILES)" wireguard)"
        docker compose up -d gameflex-wireguard
        say ""
        say "WireGuard is running. Next:"
        say "  1. Open http://$(get_setting GAMEFLEX_PUBLIC_HOST):51821 and create its administrator."
        say "     As the host, give your home's public address or a domain name pointing to it."
        say "  2. On your router, forward UDP port 51820 to this computer. Nothing else."
        say "  3. Add a device on that page, and scan its QR code with the WireGuard app."
        say "The guide: docs/remote-access.md"
        ;;
    update)
        [ -f .env ] || fail "run ./launch.sh first"
        update_files
        docker compose pull
        docker compose up -d
        docker image prune -f >/dev/null
        say "GameFlex is up to date."
        ;;
    stop)
        docker compose down
        ;;
    status)
        docker compose ps
        ;;
    reset-password)
        docker compose exec gameflex-credential /app/auth reset-root-password
        say "The administrator password is the one in .env again."
        ;;
    logs)
        shift
        services=()
        for service in "$@"; do
            services+=("gameflex-${service#gameflex-}")
        done
        docker compose logs -f --tail 100 "${services[@]}"
        ;;
    *)
        sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
        exit 2
        ;;
esac
