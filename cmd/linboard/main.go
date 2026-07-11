package main

import (
	"fmt"
	"log"
	"os"

	"github.com/foisal/linboard/internal/app"
	"github.com/foisal/linboard/internal/clipboard"
	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/hotkey"
	"github.com/foisal/linboard/internal/install"
	"github.com/foisal/linboard/internal/ipc"
	"github.com/foisal/linboard/internal/singleinstance"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[LinBoard] ")

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "toggle":
			if err := ipc.Toggle(); err != nil {
				log.Fatal(err)
			}
			return
		case "install":
			if err := install.Run(); err != nil {
				log.Fatal(err)
			}
			return
		case "install-shortcut":
			exe, err := hotkey.ExecutableForShortcut()
			if err != nil {
				log.Fatal(err)
			}
			if err := hotkey.SetupAt(exe); err != nil {
				log.Fatal(err)
			}
			report := hotkey.Verify(exe)
			hotkey.PrintVerify(report)
			if !report.Healthy() {
				os.Exit(1)
			}
			return
		case "doctor":
			exe, err := hotkey.ExecutableForShortcut()
			if err != nil {
				log.Fatal(err)
			}
			_ = hotkey.SetupAt(exe)
			report := hotkey.Verify(exe)
			hotkey.PrintVerify(report)
			clipboard.PrintPasteSetup()
			if !report.Healthy() || clipboard.SetupPasteExitCode() != 0 {
				os.Exit(1)
			}
			return
		case "setup-paste":
			clipboard.PrintPasteSetup()
			os.Exit(clipboard.SetupPasteExitCode())
		case "version", "-v", "--version":
			fmt.Printf("LinBoard %s\n", config.AppVersion)
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		log.Fatal("No display found. LinBoard requires X11 or Wayland.")
	}

	release, err := singleinstance.Acquire()
	if err != nil {
		if err == singleinstance.ErrAlreadyRunning {
			fmt.Println("LinBoard is already running — toggled history window.")
			return
		}
		log.Fatal(err)
	}
	defer release()

	application, err := app.New()
	if err != nil {
		log.Fatalf("failed to start: %v", err)
	}

	application.Run()
}

func printHelp() {
	fmt.Printf(`LinBoard %s — open-source clipboard manager for Linux

Usage:
  linboard                  Start LinBoard (system tray, background)
  linboard toggle           Show/hide history (used by Super+V shortcut)
  linboard install          Install to ~/.local/bin + autostart + Super+V
  linboard install-shortcut Register Super+V only
  linboard doctor           Check shortcut & dependencies
  linboard setup-paste      Check auto-paste (uinput) on Linux
  linboard version          Print version
  linboard help             Show this help

Hotkey: Super+V (Win+V) — like Windows clipboard history.

Supported: GNOME, KDE Plasma, XFCE, Cinnamon, MATE — X11 and Wayland.
`, config.AppVersion)
}
