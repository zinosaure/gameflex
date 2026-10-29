package docker

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
)

// buildTarget is one per-session image's build recipe — context/dockerfile
// pairs copied by hand from docker-compose.yaml's own retrogaming-image/
// streamex-switch/streamex-ps3/streamex-xbox/streamex-keytest services
// (streamex-keytest's own definition lives in the vendored streamex
// runtime's docs — docs/streamex/README.md — not the top-level compose).
// Nothing parses
// docker-compose.yaml at runtime, so a new per-console image needs an entry
// added here too — CheckImages logs which configured image has none, which
// is the signal that a target is missing from this map.
type buildTarget struct {
	context    string
	dockerfile string
	// dependsOn — another image this one's Dockerfile references via FROM
	// (built with `docker build`, not `docker compose build`, so compose's
	// own dependency ordering doesn't apply here — confirmed live: building
	// streamex-switch straight off failed with "pull access denied for
	// streamex-bin", pion Engine API has no notion of a compose-level
	// service graph). Built first, synchronously, if missing.
	dependsOn string
}

// buildTargets is keyed by image name (not by GAMEFLEX_*_IMAGE variable — a
// console-specific default like GAMEFLEX_SWITCH_IMAGE and the actual configured
// name should build the same way regardless of which env var pointed at
// it). GAMEFLEX_REMOTEPLAY_IMAGE has no entry: its compose service now tags
// gameflex-remoteplay:latest directly (image: field), so a pruned image is
// rebuilt by `docker compose build remoteplay`, not by this map.
var buildTargets = map[string]buildTarget{
	"gameflex-retrogaming:latest": {context: "/build-context/src", dockerfile: "microservices/runtimes/retrogaming/Dockerfile"},
	// streamex images build from the REPO ROOT (/build-context/root) since
	// Phase 2: their Dockerfiles now COPY src/microservices/shared/ (the shared
	// gatewayclient transport), which lives outside the streamex runtime
	// folder (vendored since 2026-08-19, ex-submodule).
	// streamex-bin is the shared skeleton (Go binaries only, FROM scratch)
	// every console Dockerfile below builds on top of via `FROM
	// streamex-bin` — see docs/streamex/README.md. It never runs as a
	// container itself, only ever exists to be a build stage.
	"streamex-bin:latest":         {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/src/Dockerfile"},
	"streamex-switch:latest":      {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/switch/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-switch-eden:latest": {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/switch-eden/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-ps3:latest":         {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/ps3/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-xbox:latest":        {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/xbox/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-ps4:latest":         {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/ps4/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-ps5:latest":         {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/ps5/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-wiiu:latest":        {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/wiiu/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-xbox360:latest":     {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/xbox360/Dockerfile", dependsOn: "streamex-bin:latest"},
	"streamex-keytest:latest":     {context: "/build-context/root", dockerfile: "src/microservices/runtimes/streamex/consoles/keytest/Dockerfile", dependsOn: "streamex-bin:latest"},
}

// buildState is one image's in-flight (or just-finished) build — the value
// half of buildsInFlight below. done closes when the build finishes; err is
// only safe to read after done is closed.
type buildState struct {
	done chan struct{}
	err  error
}

// buildsInFlight de-duplicates concurrent build requests for the SAME
// image — confirmed live: three console images (switch/xbox/keytest) all
// missing streamex-bin at once each independently kicked off their own
// build of it before this existed, three redundant copies of the same
// multi-minute build racing the same output tag. A later caller for an
// image already building — whether that's a second player launching the
// same just-pruned console, or a second console's Dockerfile that also
// depends on streamex-bin — waits on the SAME build and shares its result
// instead of starting a duplicate.
var (
	buildsInFlight   = make(map[string]*buildState)
	buildsInFlightMu sync.Mutex
)

// EnsureImageBuilt triggers a build for imageName if one isn't already
// running and this installation knows how to build it, and reports whether a
// build is on its way. It always returns immediately — the caller (CheckImages
// at boot, or a launch path that just hit a missing image) never blocks on a
// multi-minute build. Callers only learn a build finished from the log line
// (or by simply retrying their own image-missing launch a few minutes later,
// matching how the 503 message tells the player to).
//
// UNE INSTALLATION SANS SOURCES NE CONSTRUIT RIEN. La pile de dev monte ses
// contextes de build (/build-context) et sait refaire une image qu'un élagage
// a emportée. Une installation publique n'a que ses images : ses consoles se
// construisent chez l'utilisateur, par son compose. Le kernel y annonçait
// quand même « building it now », et le lancement répondait « being rebuilt —
// try again in a few minutes » : une attente qui ne finissait jamais (#136,
// relevé le 2026-09-15). Faux ici, et l'appelant dit que l'image n'est pas
// installée.
func (dockerClient *Client) EnsureImageBuilt(imageName string) bool {
	target, ok := buildTargets[imageName]
	if !ok {
		slog.Warn("session image not installed — no build recipe here, pull or build it with docker compose",
			"image", imageName)
		return false
	}
	if _, err := os.Stat(target.context); err != nil {
		slog.Warn("session image not installed — this installation has no build context, pull or build it with docker compose",
			"image", imageName, "context", target.context)
		return false
	}

	go func() {
		slog.Info("building missing session image", "image", imageName,
			"context", target.context, "dockerfile", target.dockerfile)
		if err := dockerClient.buildOnce(imageName, target); err != nil {
			slog.Error("session image build failed", "image", imageName, "error", err)
			return
		}
		slog.Info("session image build finished", "image", imageName)
	}()

	return true
}

// buildOnce runs buildImage for imageName, or — if another goroutine is
// already building the exact same image — waits for that build instead and
// returns its result. See buildsInFlight's own doc for why this matters.
func (dockerClient *Client) buildOnce(imageName string, target buildTarget) error {
	buildsInFlightMu.Lock()
	if state, ok := buildsInFlight[imageName]; ok {
		buildsInFlightMu.Unlock()
		<-state.done
		return state.err
	}
	state := &buildState{done: make(chan struct{})}
	buildsInFlight[imageName] = state
	buildsInFlightMu.Unlock()

	state.err = dockerClient.buildImage(imageName, target)

	buildsInFlightMu.Lock()
	delete(buildsInFlight, imageName)
	buildsInFlightMu.Unlock()
	close(state.done)

	return state.err
}

// buildImage runs one build to completion — a plain, synchronous call;
// buildOnce above is what de-dupes concurrent callers, EnsureImageBuilt is
// what makes the top-level call non-blocking. No timeout of its own: these
// are multi-minute CGo/libretro builds (confirmed live, retrogaming alone
// takes several minutes from a cold module cache), and the daemon itself is
// what actually enforces the process lifetime here — this call simply
// reads until the build stream ends or the caller's context is cancelled.
func (dockerClient *Client) buildImage(imageName string, target buildTarget) error {
	ctx := context.Background()

	if target.dependsOn != "" {
		if _, _, err := dockerClient.dockerClient.ImageInspectWithRaw(ctx, target.dependsOn); err != nil {
			// Missing — build it first (via buildOnce, so concurrent
			// dependents share one build instead of each starting their
			// own — see buildsInFlight's doc). Synchronous on purpose:
			// streamex-switch's own FROM streamex-bin simply cannot
			// proceed without it, and the Engine API has no notion of a
			// compose-level service graph — only `docker compose build`
			// orders services by their own dependencies, so this has to
			// happen by hand.
			depTarget, ok := buildTargets[target.dependsOn]
			if !ok {
				return fmt.Errorf("dependency %s has no build recipe", target.dependsOn)
			}
			slog.Info("building dependency image first", "image", target.dependsOn, "for", imageName)
			if err := dockerClient.buildOnce(target.dependsOn, depTarget); err != nil {
				return fmt.Errorf("build dependency %s: %w", target.dependsOn, err)
			}
		}
	}

	pipeReader, pipeWriter := io.Pipe()
	go func() {
		pipeWriter.CloseWithError(tarDirectory(target.context, pipeWriter))
	}()
	defer pipeReader.Close()

	resp, err := dockerClient.dockerClient.ImageBuild(ctx, pipeReader, types.ImageBuildOptions{
		Tags:       []string{imageName},
		Dockerfile: target.dockerfile,
		Remove:     true,
	})
	if err != nil {
		return fmt.Errorf("start build: %w", err)
	}
	defer resp.Body.Close()

	// The build can fail INSIDE the JSON message stream (a bad RUN step)
	// without ImageBuild itself ever returning an error — it only reports
	// transport-level failures (bad Dockerfile path, daemon unreachable).
	decoder := json.NewDecoder(resp.Body)
	for {
		var msg jsonmessage.JSONMessage
		if err := decoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("read build output: %w", err)
		}
		if msg.Error != nil {
			return fmt.Errorf("build step failed: %s", msg.Error.Message)
		}
	}
	return nil
}

// tarDirectory streams root's contents as a tar archive to w — a minimal,
// dependency-free stand-in for the Docker CLI's own build-context tar
// (which normally handles .dockerignore, symlinks, etc.). Deliberately
// simple: these build contexts (src/, streamex/) are plain source trees
// checked into this repo, not arbitrary user input, so the CLI's fuller
// feature set (ignore patterns, Windows path handling, hardlink dedup)
// isn't needed here. Written by hand rather than pulling in
// docker/pkg/archive — that package's TarWithOptions drags in
// github.com/moby/sys/sequential, which requires Go >= 1.24; this build
// image is pinned to 1.23, and it's one function's worth of stdlib
// archive/tar to avoid a toolchain bump for it.
func tarDirectory(root string, w io.Writer) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		// .git is the one directory worth skipping unconditionally — large,
		// irrelevant to any Dockerfile here, and by far the biggest
		// contributor to build-context size/tar time in this repo's own
		// src/ and streamex/ trees.
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
}
