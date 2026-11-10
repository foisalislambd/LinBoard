package app

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

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
	cleanupOnce  sync.Once
	cleanup      func()
	exited       atomic.Bool
	forceExit    *time.Timer
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

	// Opt into the fyne.Do threading model before any driver call. Without this,
	// Fyne logs "not been migrated" and DoAndWait from the main goroutine is
	// dispatched onto another goroutine without waiting.
	fyneapp.SetMetadata(fyne.AppMetadata{
		ID:         "com.linboard.app",
		Name:       config.AppName,
		Version:    config.AppVersion,
		Migrations: map[string]bool{"fyneDo": true},
	})
	a.fyneApp = fyneapp.NewWithID("com.linboard.app")
	a.fyneApp.SetIcon(assets.Fyne())
	clipboard.SetFyneWriter(func(text string) {
		// CopyClip runs on the Fyne UI thread (a button callback). Set the
		// clipboard directly so PasteToTarget sees it before CopyClip returns.
		// fyne.DoAndWait here would deadlock once the event loop is running.
		a.fyneApp.Clipboard().SetContent(text)
	})
	clipboard.SetFyneReader(func() string {
		var text string
		// Wayland polls from a background goroutine. On X11 the monitor does
		// not use this reader (GLFW's selection loop blocks the UI thread).
		fyne.DoAndWait(func() {
			text = a.fyneApp.Clipboard().Content()
		})
		return text
	})

	switch cfg.Theme {
	case "dark":
		a.fyneApp.Settings().SetTheme(forcedVariantTheme(theme.VariantDark))
	case "light":
		a.fyneApp.Settings().SetTheme(forcedVariantTheme(theme.VariantLight))
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
	fyne.DoAndWait(func() {
		a.history.Toggle()
	})
}

func (a *App) onClipboardChange() {
	a.updateTrayCount()
	fyne.Do(func() {
		a.history.RefreshIfVisible()
	})
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
		a.Shutdown()
	}()

	a.fyneApp.Run()
	a.exited.Store(true)
	if a.forceExit != nil {
		a.forceExit.Stop()
	}
}

// SetCleanup registers work that must run on every shutdown path, including a
// forced exit (lock file, log file). It is idempotent.
func (a *App) SetCleanup(fn func()) {
	a.cleanup = fn
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
		log.Printf("shutting down")
		a.cancel()
		if a.monitor != nil && !a.monitor.WaitTimeout(time.Second) {
			log.Printf("shutdown: clipboard monitor did not stop within 1s")
		}
		if a.hotkey != nil {
			a.hotkey.Stop()
		}
		if a.ipc != nil {
			a.ipc.Close()
		}
		clipboard.CloseUinput()
		if a.store != nil {
			_ = a.store.Close()
		}
		if a.tray != nil {
			a.tray.Quit()
		}
		a.runCleanup()
		if a.fyneApp != nil {
			fyne.Do(func() {
				a.fyneApp.Quit()
			})
		}
		// The UI thread can still be wedged inside a driver call. Never wait
		// for it forever: the lock file is already removed above.
		a.forceExit = time.AfterFunc(2*time.Second, func() {
			if a.exited.CompareAndSwap(false, true) {
				log.Printf("shutdown: UI did not exit within 2s; forcing exit")
				os.Exit(0)
			}
		})
	})
}

func (a *App) runCleanup() {
	a.cleanupOnce.Do(func() {
		if a.cleanup != nil {
			a.cleanup()
		}
	})
}
