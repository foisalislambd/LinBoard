package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

const (
	clipboardCmdTimeout = 2 * time.Second
	imageCmdTimeout     = 8 * time.Second
)

func haveCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func readTextXClip(ctx context.Context) (string, error) {
	out, err := outputFromCommand(ctx, "xclip", []string{"-selection", "clipboard", "-o"}, clipboardCmdTimeout)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func readTextXSel(ctx context.Context) (string, error) {
	out, err := outputFromCommand(ctx, "xsel", []string{"--clipboard", "--output"}, clipboardCmdTimeout)
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
	return outputFromCommand(ctx, "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}, imageCmdTimeout)
}

func writeImageXClip(data []byte) error {
	return pipeToCommand("xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}, data, imageCmdTimeout)
}

// clipboardHasImage reports whether the X11 clipboard advertises an image target.
// This avoids pulling multi-megabyte payloads on every poll when the clipboard is text.
func clipboardHasImage(ctx context.Context) bool {
	out, err := outputFromCommand(ctx, "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}, clipboardCmdTimeout)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "image/") {
			return true
		}
	}
	return false
}

func outputFromCommand(ctx context.Context, bin string, args []string, timeout time.Duration) ([]byte, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func pipeToCommand(bin string, args []string, data any, timeout time.Duration) error {
	path, err := exec.LookPath(bin)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
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
