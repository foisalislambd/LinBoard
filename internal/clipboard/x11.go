package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/foisal/linboard/internal/store"
)

const (
	clipboardCmdTimeout = 2 * time.Second
	imageCmdTimeout     = 8 * time.Second
)

// x11Mu serializes external clipboard tools. Overlapping xclip/xsel selection
// requests deadlock the owner and spin a CPU core.
var x11Mu sync.Mutex

func haveCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func readTargets(ctx context.Context) (string, error) {
	out, err := outputLimited(ctx, "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}, clipboardCmdTimeout, 64<<10)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// textTargets is the order clipboard text is requested. Plain text wins.
// Later entries cover copies that do not offer UTF8_STRING (file URIs, HTML).
var textTargets = []string{
	"UTF8_STRING",
	"text/plain;charset=utf-8",
	"text/plain",
	"STRING",
	"text/uri-list",
	"text/html",
}

// advertisedTextTargets lists text targets the owner actually offers, with
// plain text before URIs and HTML. Only advertised targets are requested, so a
// missing type cannot sit in xclip's 2s timeout.
func advertisedTextTargets(targets string) []string {
	have := make(map[string]struct{})
	for _, line := range strings.Split(targets, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || !isTextTarget(t) {
			continue
		}
		have[t] = struct{}{}
	}
	var ordered []string
	for _, t := range textTargets {
		if _, ok := have[t]; ok {
			ordered = append(ordered, t)
			delete(have, t)
		}
	}
	var rest []string
	for t := range have {
		rest = append(rest, t)
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}

// readExplicitText asks for an advertised text target and stops after MaxTextBytes.
// The default xclip target follows whatever the owner advertises, which is how
// image/png bytes were stored as text.
func readExplicitText(ctx context.Context, targets string) (string, error) {
	for _, target := range advertisedTextTargets(targets) {
		out, err := outputLimited(ctx, "xclip", []string{"-selection", "clipboard", "-t", target, "-o"}, clipboardCmdTimeout, store.MaxTextBytes)
		if errors.Is(err, store.ErrTooLarge) {
			return "", err
		}
		if err != nil || len(bytes.TrimSpace(out)) == 0 {
			continue
		}
		return string(out), nil
	}
	return "", nil
}

func readTextXSel(ctx context.Context) (string, error) {
	out, err := outputLimited(ctx, "xsel", []string{"--clipboard", "--output"}, clipboardCmdTimeout, store.MaxTextBytes)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func writeTextXClip(text string) error {
	return pipeToCommand("xclip", []string{"-selection", "clipboard", "-i"}, text, clipboardCmdTimeout)
}

func writeTextXSel(text string) error {
	return pipeToCommand("xsel", []string{"--clipboard", "--input"}, text, clipboardCmdTimeout)
}

func readImageXClip(ctx context.Context) ([]byte, error) {
	return outputLimited(ctx, "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}, imageCmdTimeout, store.MaxImageBytes)
}

func writeImageXClip(data []byte) error {
	return pipeToCommand("xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}, data, imageCmdTimeout)
}

// outputLimited runs a clipboard tool and keeps at most maxBytes of stdout.
// The process group is killed when the context ends or the payload is oversize,
// so a stuck xclip cannot pin a core or block shutdown.
func outputLimited(ctx context.Context, bin string, args []string, timeout time.Duration, maxBytes int) ([]byte, error) {
	x11Mu.Lock()
	defer x11Mu.Unlock()

	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	prepareCommand(cmd)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	buf, readErr := readCapped(stdout, maxBytes)
	if errors.Is(readErr, store.ErrTooLarge) {
		cancel()
		_ = stdout.Close()
		_ = cmd.Wait()
		return nil, store.ErrTooLarge
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		// xclip/xsel exit non-zero when the requested target is absent.
		// That is an empty clipboard, not a stored payload.
		if len(buf) == 0 {
			return nil, waitErr
		}
		return buf, nil
	}
	return buf, nil
}

func pipeToCommand(bin string, args []string, data any, timeout time.Duration) error {
	x11Mu.Lock()
	defer x11Mu.Unlock()

	path, err := exec.LookPath(bin)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	prepareCommand(cmd)
	switch v := data.(type) {
	case string:
		cmd.Stdin = strings.NewReader(v)
	case []byte:
		cmd.Stdin = bytes.NewReader(v)
	default:
		return fmt.Errorf("unsupported pipe data type")
	}
	if err := cmd.Run(); err != nil {
		log.Printf("clipboard (%s): %v", bin, err)
		return err
	}
	return nil
}
