package app

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"github.com/foisal/linboard/internal/assets"
	"github.com/foisal/linboard/internal/clipboard"
	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/hotkey"
	"github.com/foisal/linboard/internal/ipc"
	"github.com/foisal/linboard/internal/platform"
	"github.com/foisal/linboard/internal/store"
	"github.com/foisal/linboard/internal/tray"
	"github.com/foisal/linboard/internal/ui"
)

type App struct {
	cfg          *config.Config
	fyneApp      fyne.App
	store        *store.Store
	history      *ui.HistoryWindow
	monitor      *clipboard.Monitor
	hotkey       *hotkey.Manager
	tray         *tray.Tray
	ipc          *ipc.Server
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownOnce sync.Once
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	s, err := store.Open(cfg.MaxHistory)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	a := &App{
		cfg:    cfg,
		store:  s,
		ctx:    ctx,
		cancel: cancel,
	}

	a.fyneApp = fyneapp.NewWithID("com.linboard.app")
	a.fyneApp.SetIcon(assets.Fyne())
	clipboard.SetFyneWriter(func(text string) {
		// CopyClip runs on the Fyne main thread — set synchronously so PasteToTarget
		// sees the new clipboard contents immediately after CopyClip returns.
		a.fyneApp.Clipboard().SetContent(text)
	})
	clipboard.SetFyneReader(func() string {
		var text string
		// Monitor polls from a background goroutine — must block until read completes.
		fyne.DoAndWait(func() {
			text = a.fyneApp.Clipboard().Content()
		})
		return text
	})

	switch cfg.Theme {
	case "dark":
		a.fyneApp.Settings().SetTheme(theme.DarkTheme())
	case "light":
		a.fyneApp.Settings().SetTheme(theme.LightTheme())
	default:
		// "system" — leave Fyne default so the platform preference is used where supported.
	}

	a.history = ui.NewHistoryWindow(a.fyneApp, s, cfg)
	a.monitor = clipboard.NewMonitor(s)
	a.hotkey = hotkey.New()
	a.tray = tray.New()

	return a, nil
}

func (a *App) showHistory() {
	a.fyneApp.Driver().DoFromGoroutine(func() {
		a.history.Toggle()
	}, true)
}

func (a *App) onClipboardChange() {
	a.updateTrayCount()
	a.fyneApp.Driver().DoFromGoroutine(func() {
		a.history.RefreshIfVisible()
	}, false)
}

func (a *App) Run() {
	log.Printf("LinBoard %s — %s / %s (clipboard read: %s, copy: %s, paste: %s)",
		config.AppVersion, platform.SessionDescription(), platform.DesktopName(),
		clipboard.ReadToolName(), clipboard.CopyToolName(), clipboard.PasteToolName())
	if clipboard.SessionNeedsRelogin() {
		log.Printf("auto-paste: input group not active in this session — log out/in, or: sg input -c linboard")
	}

	// Start IPC first so Super+V works while the rest of the app initializes.
	ipcSrv, err := ipc.StartServer(a.showHistory)
	if err != nil {
		log.Printf("ipc server failed: %v", err)
	} else {
		a.ipc = ipcSrv
	}

	a.monitor.OnChange(a.onClipboardChange)
	a.monitor.Start(a.ctx)

	a.hotkey.OnPress(a.showHistory)

	a.tray.OnShow(a.showHistory)
	a.tray.OnClear(func() {
		if err := a.store.ClearUnpinned(); err != nil {
			log.Printf("clear history: %v", err)
		}
		a.updateTrayCount()
	})
	a.tray.OnQuit(func() {
		a.Shutdown()
	})

	go func() {
		if err := a.hotkey.Start(); err != nil {
			log.Printf("hotkey: %v", err)
			log.Printf("tray → Show History, or run: linboard install-shortcut")
		}
	}()

	go a.tray.Run(func() {
		a.updateTrayCount()
		if !a.cfg.StartMinimized {
			a.showHistory()
		}
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		a.tray.Quit()
		a.Shutdown()
	}()

	a.fyneApp.Run()
}

func (a *App) updateTrayCount() {
	n, err := a.store.Count()
	if err != nil {
		return
	}
	a.tray.SetCount(n)
}

func (a *App) Shutdown() {
	a.shutdownOnce.Do(func() {
		a.cancel()
		a.hotkey.Stop()
		if a.ipc != nil {
			a.ipc.Close()
		}
		_ = a.store.Close()
		a.fyneApp.Driver().DoFromGoroutine(func() {
			a.fyneApp.Quit()
		}, false)
	})
}
