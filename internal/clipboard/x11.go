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

const clipboardCmdTimeout = 2 * time.Second

func haveCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func readTextXClip() (string, error) {
	out, err := outputFromCommand("xclip", []string{"-selection", "clipboard", "-o"})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func readTextXSel() (string, error) {
	out, err := outputFromCommand("xsel", []string{"--clipboard", "--output"})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func writeTextXClip(text string) error {
	return pipeToCommand("xclip", []string{"-selection", "clipboard", "-i"}, text)
}

func writeTextXSel(text string) error {
	return pipeToCommand("xsel", []string{"--clipboard", "--input"}, text)
}

func readImageXClip() ([]byte, error) {
	return outputFromCommand("xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"})
}

func writeImageXClip(data []byte) error {
	return pipeToCommand("xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}, data)
}

func outputFromCommand(bin string, args []string) ([]byte, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), clipboardCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func pipeToCommand(bin string, args []string, data any) error {
	path, err := exec.LookPath(bin)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), clipboardCmdTimeout)
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
