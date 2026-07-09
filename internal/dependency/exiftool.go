package dependency

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
)

// Exiftool is a long-lived exiftool process in -stay_open mode. Spawning exiftool
// per file costs ~200ms of Perl startup; the daemon amortizes that to near-zero
// across a scan (impl/07 acceptance: ≥100 reads/s). One process serves both XMP
// reads and writes; calls are serialized over a single stdin/stdout pipe pair.
//
// Transport: exiftool reads arguments from stdin (launched with `-@ -`), one per
// line, and processes a batch when it sees `-execute<n>`, after which it prints
// `{ready<n>}` on stdout. We merge stderr into stdout (one os.Pipe feeding both
// cmd.Stdout and cmd.Stderr) so a single reader and a single sentinel delimit each
// response — warnings and errors arrive inline before the marker. Execute returns
// that raw blob; interpreting it (JSON for reads, "N image files updated" for
// writes) belongs to the caller (internal/xmp), not this transport.
//
// Orphan safety: exiftool self-exits when our stdin pipe closes (parent death →
// EOF), which is the free, cross-platform layer and the only process we spawn.
// ponytail: layer-3 defense (Linux Pdeathsig / Windows Job Object, impl/07 §Orphan)
// is deferred — the EOF convention covers a clean exit and a normal crash; add the
// per-OS build-tagged spawn files if a hard SIGKILL race ever leaves a stray PID.
type ExiftoolDaemon struct {
	logger *log.Logger

	mu     sync.Mutex // serializes Execute; one pipe, one conversation at a time
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output *bufio.Reader
	seq    int
	closed bool
}

// StartExiftool launches the daemon for a discovered, available tool. It returns
// an error if the status is not StateFound (callers should check Status.Available
// first and degrade) or if the process fails to spawn.
func StartExiftool(status Status, logger *log.Logger) (*ExiftoolDaemon, error) {
	if !status.Available() {
		return nil, fmt.Errorf("exiftool unavailable: %s", status.State)
	}
	if logger == nil {
		logger = log.Default()
	}

	// -stay_open True keeps the process alive; -@ - reads arguments from stdin.
	// -common_args would apply flags to every command; we keep per-call args explicit.
	cmd := exec.Command(status.Path, "-stay_open", "True", "-@", "-") //nolint:gosec // path verified by dependency check

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool: stdin pipe: %w", err)
	}
	// Merge stdout+stderr onto one pipe so a single {ready<n>} marker terminates
	// each response regardless of which stream exiftool wrote to.
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool: output pipe: %w", err)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, fmt.Errorf("exiftool: start: %w", err)
	}
	// The child holds its own dup of the write end; close ours so EOF propagates
	// to the reader once the child exits.
	_ = writer.Close()

	logger.Info("dependency: exiftool daemon started", "path", status.Path, "pid", cmd.Process.Pid, "version", status.Version)
	return &ExiftoolDaemon{
		logger: logger,
		cmd:    cmd,
		stdin:  stdin,
		output: bufio.NewReader(reader),
	}, nil
}

// ErrDaemonClosed is returned by Execute after Close.
var ErrDaemonClosed = errors.New("exiftool daemon closed")

// Execute runs one exiftool command and returns everything it printed (stdout and
// stderr merged) up to the ready marker. Calls are serialized. The ctx is honored
// only at entry — exiftool's stay_open pipe has no per-command cancellation, and a
// single metadata op is sub-second; a wedged process is the dependency timeout's
// job (deferred with the one-shot path), not this method's.
func (e *ExiftoolDaemon) Execute(ctx context.Context, args ...string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, ErrDaemonClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	e.seq++
	marker := "{ready" + strconv.Itoa(e.seq) + "}"

	// Write each argument on its own line, then trigger the batch. exiftool's
	// argfile format is strictly one argument per line — no quoting, no splitting.
	var request []byte
	for _, arg := range args {
		request = append(request, arg...)
		request = append(request, '\n')
	}
	request = append(request, "-execute"+strconv.Itoa(e.seq)+"\n"...)

	if _, err := e.stdin.Write(request); err != nil {
		return nil, fmt.Errorf("exiftool: write command: %w", err)
	}

	response, err := readUntilMarker(e.output, marker)
	if err != nil {
		return nil, fmt.Errorf("exiftool: read response: %w", err)
	}
	e.logger.Debug("dependency: exiftool command", "seq", e.seq, "args", len(args), "bytes", len(response))
	return response, nil
}

// Close stops the daemon gracefully: `-stay_open False` tells exiftool to exit its
// loop, then closing stdin sends EOF as the backstop. It waits for the process so
// no zombie is left.
func (e *ExiftoolDaemon) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true

	// Best-effort graceful stop; ignore write errors (the process may already be
	// gone) and fall through to closing stdin, which forces exit via EOF.
	_, _ = e.stdin.Write([]byte("-stay_open\nFalse\n"))
	_ = e.stdin.Close()

	err := e.cmd.Wait()
	e.logger.Info("dependency: exiftool daemon stopped", "err", err)
	return err
}

// readUntilMarker reads lines until one equals the ready marker, returning
// everything before it. The marker line itself is consumed and dropped. An EOF
// before the marker means the daemon died mid-command.
func readUntilMarker(reader *bufio.Reader, marker string) ([]byte, error) {
	var accumulated []byte
	for {
		line, err := reader.ReadString('\n')
		// exiftool prints the marker followed by a newline; on some platforms the
		// final line may lack it, so check the trimmed content before the EOF test.
		if strings.TrimRight(line, "\r\n") == marker {
			return accumulated, nil
		}
		accumulated = append(accumulated, line...)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return accumulated, fmt.Errorf("daemon exited before ready marker %q", marker)
			}
			return accumulated, err
		}
	}
}
