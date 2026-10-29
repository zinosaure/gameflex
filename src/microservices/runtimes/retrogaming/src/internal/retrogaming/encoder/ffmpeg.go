package encoder

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// scanLinesOrCR is bufio.ScanLines but also treats a bare '\r' as a line
// terminator. ffmpeg's own default progress stats ("frame=... fps=...
// time=... bitrate=...") update in place using '\r' with no trailing '\n' —
// bufio.Scanner's stock ScanLines only terminates a token on '\n', so the
// entire running total of those updates accumulates into a single, ever-
// growing in-flight token for as long as the process runs. Once it exceeds
// bufio.MaxScanTokenSize (64KB), Scan() fails and returns false, silently
// ending the `for sc.Scan()` loop — nobody drains e.stderr again for the
// rest of the session, its pipe fills up, and ffmpeg's own write() to its
// OWN stderr blocks (synchronously, inline with its frame processing),
// stalling the entire encode pipeline. Confirmed via a real report: retro-
// gaming sessions got progressively laggier for ANY core the longer they
// ran, and via this exact garbled single-line log entry seen earlier this
// session (a wall of concatenated "size=...time=...bitrate=..." with no
// line breaks) — it takes a few minutes of continuous stats updates to
// accumulate past 64KB, which is exactly the "gets worse over time,
// independent of core" shape reported.
//
// -loglevel error -nostats below removes the root cause (ffmpeg no longer
// writes this at all); this split function is a second line of defense so
// any OTHER \r-based verbose output can't silently repeat the same failure.
func scanLinesOrCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	idxNL := bytes.IndexByte(data, '\n')
	idxCR := bytes.IndexByte(data, '\r')
	idx := -1
	switch {
	case idxNL >= 0 && (idxCR < 0 || idxNL < idxCR):
		idx = idxNL
	case idxCR >= 0:
		idx = idxCR
	}
	if idx >= 0 {
		return idx + 1, data[0:idx], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

type VideoEncoder struct {
	width, height int
	fps           float64
	bitrate       int
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	stderr        io.ReadCloser
	started       atomic.Bool
	mu            sync.Mutex
	wg            sync.WaitGroup
	Output        chan []byte
	stopCh        chan struct{}
	// Dropped counts NALs discarded because Output was full (see readOutput).
	// Any single dropped NAL breaks x264's reference chain for every
	// subsequent P-frame that depends on it — the browser decoder can't
	// resync until the next keyframe, which at keyint=60/60fps is up to a
	// full second away. A non-zero count here is the direct explanation for
	// "press a button, image doesn't react for ~1s" symptoms.
	Dropped atomic.Uint64
}

// AutoBitrate — le débit déduit de l'image, en kbit/s.
//
// Il était FIXE à 2000, quelle que soit la taille. Une NES (256×240) en
// recevait cinq fois trop et une PlayStation (640×480) pas assez : sur de
// l'art pixelisé, le plafond écrêtait et l'image devenait médiocre (signalé
// le 2026-09-05, sur Crash Team Racing).
//
// La formule est celle que le téléphone emploie déjà (GatewayEncoder.java) —
// des bits par pixel et par seconde — mais avec un COEFFICIENT PLUS ÉLEVÉ, et
// ce n'est pas un détail : là-bas c'est un encodeur MATÉRIEL en profil élevé,
// ici c'est libx264 en `baseline` + `ultrafast`, la combinaison la moins
// efficace qui soit. Choisie exprès (compatibilité des décodeurs, latence),
// elle a besoin de plus de débit pour le même résultat.
//
// PLANCHER à l'ancienne valeur : rien ne peut devenir pire qu'aujourd'hui.
// PLAFOND parce qu'un débit qu'aucun réseau ne suit ne sert personne, et que
// le tampon VBV en dépend (voir -maxrate/-bufsize).
func AutoBitrate(width, height int, fps float64) int {
	if width <= 0 || height <= 0 || fps <= 0 {
		return autoBitrateFloor
	}

	kbps := int(float64(width) * float64(height) * fps * autoBitsPerPixel / 1000)
	switch {
	case kbps < autoBitrateFloor:
		return autoBitrateFloor
	case kbps > autoBitrateCeiling:
		return autoBitrateCeiling
	}

	return kbps
}

const (
	// Bits par pixel et par image. 0,07 suffit à un encodeur matériel ;
	// `baseline`+`ultrafast` en demande environ le triple.
	autoBitsPerPixel = 0.20
	// L'ancienne valeur fixe — devenue le plancher, pour que ce changement
	// ne puisse qu'améliorer.
	autoBitrateFloor = 2000
	// Au-delà, c'est le réseau qui lâche avant que l'image ne gagne.
	autoBitrateCeiling = 8000
)

// NewVideoEncoder — `bitrate` à 0 (ou moins) demande le débit AUTOMATIQUE,
// déduit de la taille et de la cadence. C'est ce que tout appelant devrait
// passer : une valeur ronde ne peut être juste que pour une seule taille.
func NewVideoEncoder(width, height int, fps float64, bitrate int) *VideoEncoder {
	if bitrate <= 0 {
		bitrate = AutoBitrate(width, height, fps)
	}

	return &VideoEncoder{
		width: width, height: height,
		fps: fps, bitrate: bitrate,
		Output: make(chan []byte, 64),
		stopCh: make(chan struct{}),
	}
}

func (e *VideoEncoder) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started.Load() {
		return fmt.Errorf("already started")
	}
	// ⚠️ CRITICAL x264 PARAMETERS for real-time WebRTC streaming:
	//
	// threads=1, sliced-threads=0:
	//   - Forces single-threaded encoding with NO slice-based parallelism
	//   - CRITICAL: Without this, x264 splits each frame into 3 slices (VCL NALs)
	//   - Each slice would be sent as separate RTP sample → breaks decoder
	//   - Symptom: corrupted video, only keyframes display
	//   - Performance: ~30-40% CPU for 256×240@60fps (acceptable for streaming)
	//
	// rc-lookahead=0, sync-lookahead=0:
	//   - Disables future frame analysis (reduces latency by ~50-100ms)
	//   - Trade-off: slightly lower compression efficiency
	//
	// keyint=15, min-keyint=15, scenecut=0:
	//   - Forces keyframe every 15 frames (250ms @ 60fps) — was 60 (1 full
	//     second). Output <- nal in readOutput() below drops a NAL outright
	//     if the consumer briefly falls behind (channel full, non-blocking
	//     select); losing even one P-frame breaks x264's reference chain for
	//     every frame after it, so the browser decoder shows a frozen/
	//     corrupted picture until the NEXT keyframe arrives. At keyint=60
	//     that's up to 1s of visibly "stuck" video after a single dropped
	//     NAL — exactly the "press a button, ~1s before I see it react"
	//     symptom reported on PS1 (640×480 puts far more load on the
	//     single-threaded encoder than the already-OK lighter cores, making
	//     an occasional drop more likely there). keyint=15 caps the same
	//     failure at 250ms instead — a real fix needs the drop itself gone
	//     (see the Dropped counter/log below to confirm how often it's
	//     actually happening), this only bounds the damage when it does.
	//   - New viewers join within 250ms max (down from 1s) as a side effect
	//   - scenecut=0 prevents adaptive keyframes (predictable timing)
	//
	// -loglevel error -nostats: without this, ffmpeg's default '\r'-updated
	// progress stats eventually kill the stderr-draining goroutine below and
	// stall the whole encoder — see scanLinesOrCR's own doc for the full
	// failure chain. Shared by both paths below.
	inputArgs := []string{
		"-loglevel", "error", "-nostats",
		"-f", "rawvideo", "-pixel_format", "bgra",
		"-video_size", fmt.Sprintf("%dx%d", e.width, e.height),
		"-framerate", fmt.Sprintf("%.2f", e.fps),
		"-fflags", "nobuffer", // Disable input buffering
		"-flags", "low_delay", // Minimize encoding delay
	}

	var args []string
	args = append(args, inputArgs...)
	args = append(args,
		// ⚠️ CRITICAL x264 PARAMETERS for real-time WebRTC streaming:
		//
		// threads=1, sliced-threads=0:
		//   - Forces single-threaded encoding with NO slice-based parallelism
		//   - CRITICAL: Without this, x264 splits each frame into 3 slices (VCL NALs)
		//   - Each slice would be sent as separate RTP sample → breaks decoder
		//   - Symptom: corrupted video, only keyframes display
		//   - Performance: ~30-40% CPU for 256×240@60fps (acceptable for streaming)
		//
		// rc-lookahead=0, sync-lookahead=0:
		//   - Disables future frame analysis (reduces latency by ~50-100ms)
		//   - Trade-off: slightly lower compression efficiency
		//
		// keyint=15, min-keyint=15, scenecut=0:
		//   - Forces keyframe every 15 frames (250ms @ 60fps) — was 60 (1 full
		//     second). Output <- nal in readOutput() below drops a NAL outright
		//     if the consumer briefly falls behind (channel full, non-blocking
		//     select); losing even one P-frame breaks x264's reference chain for
		//     every frame after it, so the browser decoder shows a frozen/
		//     corrupted picture until the NEXT keyframe arrives. At keyint=60
		//     that's up to 1s of visibly "stuck" video after a single dropped
		//     NAL — exactly the "press a button, ~1s before I see it react"
		//     symptom reported on PS1 (640×480 puts far more load on the
		//     single-threaded encoder than the already-OK lighter cores, making
		//     an occasional drop more likely there). keyint=15 caps the same
		//     failure at 250ms instead — a real fix needs the drop itself gone
		//     (see the Dropped counter/log below to confirm how often it's
		//     actually happening), this only bounds the damage when it does.
		//   - New viewers join within 250ms max (down from 1s) as a side effect
		//   - scenecut=0 prevents adaptive keyframes (predictable timing)
		"-i", "pipe:0", "-an", "-f", "h264",
		"-vcodec", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
		"-crf", "23", "-bf", "0", "-g", "15",
		"-x264-params", "keyint=15:min-keyint=15:scenecut=0:rc-lookahead=0:sync-lookahead=0:sliced-threads=0:threads=1",
		// PROFIL `high`, PLUS `baseline` — décidé le 2026-09-05.
		//
		// `baseline` était là pour un vieux Chromecast qui ne décodait
		// rien d'autre. Il n'est plus dans le parc, et ce profil coûtait
		// cher : pas de CABAC (l'entropie arithmétique, ~10 à 15 % de
		// mieux à qualité égale), pas de transformée 8×8. C'est le plus
		// gros écart de qualité avec le téléphone, qui encode en matériel
		// sans cette contrainte.
		//
		// Ce que `high` NE change PAS, et c'est voulu : les trames B
		// restent interdites (`-bf 0`), parce que c'est la LATENCE qui
		// les interdit, pas le profil — une trame B se code d'après une
		// image future, donc il faut l'attendre.
		//
		// Niveau 4.0 comme le chemin matériel : de la marge pour du
		// 640×480@60, et un décodeur qui refuse un niveau trop bas rend
		// une image NOIRE sans le dire (vécu le 2026-08-13).
		"-profile:v", "high", "-level", "4.0", "-pix_fmt", "yuv420p",
		"-b:v", fmt.Sprintf("%dk", e.bitrate),
		// -maxrate/-bufsize (VBV) — without these, -crf leaves x264 free to
		// produce a frame of ANY size regardless of -b:v (that flag alone
		// does nothing to cap peaks under CRF-based rate control); -b:v is
		// only honored as a real ceiling once paired with a VBV buffer.
		// Confirmed via real testing: retro pixel art (dithering, noisy
		// detail — notoriously hard for H.264 to compress, worse still on
		// the least-efficient `baseline` profile picked above for decoder
		// compatibility) plus a forced keyframe every 250ms (-g 15) could
		// spike well past the nominal bitrate on any busy frame with
		// nothing to stop it — an old Chromecast decoding via a Custom Web
		// Receiver's plain WebRTC path (no hardware fast-path the way
		// Chrome's own tab-mirroring encoder gets) choked on exactly those
		// spikes while every other viewer had enough headroom to absorb
		// them unnoticed. bufsize == maxrate keeps the cap tight, in
		// keeping with -tune zerolatency's whole point.
		//
		// LE TAMPON VAUT DEUX FOIS LE DÉBIT depuis le 2026-09-05.
		//
		// Il valait exactement le débit — « bufsize == maxrate keeps the
		// cap tight » — pour ce même vieux Chromecast. Serré à ce
		// point, le tampon oblige CHAQUE image à tenir dans le budget
		// d'une seconde de flux : une image chargée ne peut pas
		// emprunter aux suivantes, et sur de l'art pixelisé c'est la
		// plupart d'entre elles. Le débit moyen reste tenu ; c'est
		// seulement la marge par image qui s'ouvre.
		"-maxrate", fmt.Sprintf("%dk", e.bitrate),
		"-bufsize", fmt.Sprintf("%dk", e.bitrate*2),
		"pipe:1",
	)
	e.cmd = exec.Command("ffmpeg", args...)
	var err error
	e.stdin, _ = e.cmd.StdinPipe()
	e.stdout, _ = e.cmd.StdoutPipe()
	e.stderr, _ = e.cmd.StderrPipe()
	if err = e.cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	e.started.Store(true)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		sc := bufio.NewScanner(e.stderr)
		sc.Split(scanLinesOrCR)
		for sc.Scan() {
			log.Printf("[ffmpeg-video] %s", sc.Text())
		}
	}()
	e.wg.Add(1)
	go e.readOutput()
	log.Printf("Video encoder started: %dx%d @ %.1ffps, software (libx264)", e.width, e.height, e.fps)
	return nil
}

func (e *VideoEncoder) readOutput() {
	defer e.wg.Done()
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	startCode3 := []byte{0x00, 0x00, 0x01}
	var buf bytes.Buffer
	tmp := make([]byte, 65536)
	for {
		select {
		case <-e.stopCh:
			return
		default:
		}
		n, err := e.stdout.Read(tmp)
		if err != nil {
			if err != io.EOF {
				log.Printf("Video read error: %v", err)
			}
			return
		}
		buf.Write(tmp[:n])
		for {
			data := buf.Bytes()
			idx4 := bytes.Index(data, startCode)
			idx3 := bytes.Index(data, startCode3)
			startIdx, startLen := -1, 0
			if idx4 >= 0 && (idx3 < 0 || idx4 < idx3) {
				startIdx, startLen = idx4, 4
			} else if idx3 >= 0 {
				startIdx, startLen = idx3, 3
			}
			if startIdx < 0 {
				break
			}

			searchFrom := startIdx + startLen
			nextIdx4 := bytes.Index(data[searchFrom:], startCode)
			nextIdx3 := bytes.Index(data[searchFrom:], startCode3)
			nextIdx := -1
			if nextIdx4 >= 0 && (nextIdx3 < 0 || nextIdx4 < nextIdx3) {
				nextIdx = searchFrom + nextIdx4
			} else if nextIdx3 >= 0 {
				nextIdx = searchFrom + nextIdx3
			}
			if nextIdx < 0 {
				break
			}

			nal := make([]byte, nextIdx-startIdx)
			copy(nal, data[startIdx:nextIdx])
			select {
			case e.Output <- nal:
			case <-e.stopCh:
				return
			default:
				n := e.Dropped.Add(1)
				if n <= 5 || n%50 == 0 {
					log.Printf("[video-out] DROPPED NAL #%d (Output channel full, consumer falling behind) — decode chain broken until next keyframe", n)
				}
			}
			buf.Next(nextIdx)
		}
	}
}

func (e *VideoEncoder) EncodeFrame(data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started.Load() {
		return fmt.Errorf("not started")
	}
	_, err := e.stdin.Write(data)
	return err
}

func (e *VideoEncoder) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started.Load() {
		return nil
	}
	close(e.stopCh)
	e.started.Store(false)
	e.stdin.Close()
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		e.cmd.Process.Kill()
	}
	e.cmd.Wait()
	log.Println("Video encoder stopped")
	return nil
}

// ---- Audio ----

type AudioEncoder struct {
	sampleRate, channels int
	cmd                  *exec.Cmd
	stdin                io.WriteCloser
	stdout               io.ReadCloser
	stderr               io.ReadCloser
	started              atomic.Bool
	mu                   sync.Mutex
	wg                   sync.WaitGroup
	Output               chan []byte
	stopCh               chan struct{}
}

func NewAudioEncoder(sampleRate, channels int) *AudioEncoder {
	return &AudioEncoder{
		sampleRate: sampleRate, channels: channels,
		Output: make(chan []byte, 128),
		stopCh: make(chan struct{}),
	}
}

func (e *AudioEncoder) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started.Load() {
		return fmt.Errorf("already started")
	}
	args := []string{
		// See the video encoder's own identical flags for why — same
		// failure mode applies here (scanLinesOrCR's doc has the full
		// chain), and this is the encoder that actually produced the
		// garbled single-line log entry that first surfaced this bug.
		"-loglevel", "error", "-nostats",
		"-f", "s16le", "-ar", fmt.Sprintf("%d", e.sampleRate), "-ac", fmt.Sprintf("%d", e.channels),
		"-i", "pipe:0", "-f", "opus", "-acodec", "libopus",
		// 10ms (not Opus's more common 20ms) — halves the baseline latency
		// that comes from simply having to accumulate a full frame's worth
		// of audio before it can be encoded at all; must stay consistent
		// with audioBufMax and packetDuration in stream_session.go, and
		// page_duration below.
		"-b:a", "64k", "-application", "lowdelay", "-frame_duration", "10",
		"-vbr", "off", "-compression_level", "0",
		// Unlike the video path (-f h264, a raw elementary bitstream with
		// no muxer buffering), this goes through ffmpeg's actual Ogg muxer
		// — its AVIOContext has its own internal write buffer (tens of KB
		// by default) that only gets flushed to the pipe once full,
		// independent of anything on the Go side. At 64kbps that's several
		// SECONDS of audio silently held back before the first bytes even
		// reach us — a large, constant lag from the very start of a
		// session, not something backpressure/dropping on our end could
		// ever fix. -flush_packets forces a flush after every packet
		// instead.
		"-flush_packets", "1",
		// -flush_packets alone wasn't enough — the ogg muxer's OWN page
		// batching is a separate, much bigger delay: ffmpeg's default
		// `page_duration` is 1000000µs (1 full second). It won't finalize
		// (and therefore won't emit/flush) an Ogg page until it has ~1s of
		// audio for it, regardless of -flush_packets — that flag only
		// controls when an already-finalized page gets written to the
		// pipe, not how long ffmpeg waits before finalizing one in the
		// first place. Set to 20ms to match our own packet/frame duration,
		// so pages close almost immediately instead of batching ~1s at a
		// time.
		"-page_duration", "10000",
		"pipe:1",
	}
	e.cmd = exec.Command("ffmpeg", args...)
	e.stdin, _ = e.cmd.StdinPipe()
	e.stdout, _ = e.cmd.StdoutPipe()
	e.stderr, _ = e.cmd.StderrPipe()
	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg audio: %w", err)
	}
	e.started.Store(true)
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		sc := bufio.NewScanner(e.stderr)
		sc.Split(scanLinesOrCR)
		for sc.Scan() {
			log.Printf("[ffmpeg-audio] %s", sc.Text())
		}
	}()
	e.wg.Add(1)
	go e.readOutput()
	log.Printf("Audio encoder started: %d Hz", e.sampleRate)
	return nil
}

// readOutput parses ffmpeg's `-f opus` stdout as the Ogg container it
// actually is (confirmed by hex-dumping it: starts with "OggS", an
// OpusHead identification packet, then an OpusTags comment packet — Opus
// has no bare/headerless file format, Ogg is the only one, per RFC 7845).
//
// The original (pre-port) version of this code just forwarded raw 4096-byte
// reads from stdout directly as if each chunk were one complete Opus RTP
// sample — silently broken: chunk boundaries don't align with Ogg page/
// packet boundaries, and the first chunk(s) contain Ogg page headers +
// OpusHead/OpusTags metadata mixed with audio, none of which are valid
// standalone Opus frames. The browser's decoder never got a valid frame,
// so the WebRTC audio track stayed on `muted` forever (confirmed via
// `MediaStreamTrack.muted` staying true throughout a live session) even
// though ffmpeg itself was encoding real audio the whole time (its own
// stderr progress counters kept climbing) — everything downstream of
// ffmpeg's stdout was reading nonsense.
func (e *AudioEncoder) readOutput() {
	defer e.wg.Done()
	pages := newOggPageReader(e.stdout)
	opusHead := []byte("OpusHead")
	opusTags := []byte("OpusTags")
	for {
		select {
		case <-e.stopCh:
			return
		default:
		}
		packets, err := pages.nextPackets()
		if err != nil {
			if err != io.EOF {
				log.Printf("Audio read error: %v", err)
			}
			return
		}
		for _, pkt := range packets {
			// RFC 7845 §3: identification + comment header packets always
			// precede any audio — skip by content, not by page/packet
			// count, since the comment header is allowed to span multiple
			// pages depending on its size.
			if bytes.HasPrefix(pkt, opusHead) || bytes.HasPrefix(pkt, opusTags) {
				continue
			}
			select {
			case e.Output <- pkt:
			case <-e.stopCh:
				return
			default:
			}
		}
	}
}

// oggPageReader reconstructs Ogg logical-bitstream packets from a raw byte
// stream of Ogg pages (see RFC 3533). Handles packets that span multiple
// pages (the "continuation" header flag + a lacing value of 255 meaning
// "more of this packet follows"), which Opus audio frames can occasionally
// do even though most fit in a single page at typical bitrates.
type oggPageReader struct {
	r       *bufio.Reader
	pending []byte // in-progress packet continuing from the previous page
}

func newOggPageReader(r io.Reader) *oggPageReader {
	return &oggPageReader{r: bufio.NewReaderSize(r, 65536)}
}

// nextPackets reads exactly one Ogg page and returns the packets it
// completes. A packet that continues into the next page is buffered
// internally (in `pending`) and not returned until a later page finishes it.
func (o *oggPageReader) nextPackets() ([][]byte, error) {
	header := make([]byte, 27)
	if _, err := io.ReadFull(o.r, header); err != nil {
		return nil, err
	}
	if !bytes.Equal(header[0:4], []byte("OggS")) {
		return nil, fmt.Errorf("not an Ogg page (bad capture pattern)")
	}
	segCount := int(header[26])
	segTable := make([]byte, segCount)
	if _, err := io.ReadFull(o.r, segTable); err != nil {
		return nil, err
	}

	// header[5] bit 0x01 marks "this page starts with a continuation of the
	// previous page's last packet" — no separate check needed here: `cur`
	// below already carries that in-progress packet forward via `o.pending`
	// regardless (nil if there was nothing pending).
	var packets [][]byte
	cur := o.pending
	o.pending = nil
	for _, segLen := range segTable {
		if segLen > 0 {
			buf := make([]byte, segLen)
			if _, err := io.ReadFull(o.r, buf); err != nil {
				return nil, err
			}
			cur = append(cur, buf...)
		}
		if segLen < 255 {
			// A lacing value under 255 always terminates a packet.
			packets = append(packets, cur)
			cur = nil
		}
		// A lacing value of exactly 255 means the packet continues in the
		// next segment (or, if this was the last entry in the table, the
		// next page) — keep accumulating into `cur`.
	}
	if cur != nil {
		o.pending = cur
	}
	return packets, nil
}

func (e *AudioEncoder) EncodeSamples(data []int16) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started.Load() {
		return fmt.Errorf("not started")
	}
	buf := make([]byte, len(data)*2)
	for i, v := range data {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	_, err := e.stdin.Write(buf)
	return err
}

func (e *AudioEncoder) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started.Load() {
		return nil
	}
	close(e.stopCh)
	e.started.Store(false)
	e.stdin.Close()
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		e.cmd.Process.Kill()
	}
	e.cmd.Wait()
	log.Println("Audio encoder stopped")
	return nil
}
