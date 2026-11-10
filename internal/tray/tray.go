package tray

import (
	"fmt"
	"log"
	"sync"

	"fyne.io/systray"

	"github.com/foisal/linboard/internal/assets"
	"github.com/foisal/linboard/internal/config"
)

type Tray struct {
	onShow    func()
	onClear   func()
	onQuit    func()
	countItem *systray.MenuItem
	quit      chan struct{}
	quitOnce  sync.Once
}

func New() *Tray {
	return &Tray{quit: make(chan struct{})}
}

func (t *Tray) OnShow(fn func())  { t.onShow = fn }
func (t *Tray) OnClear(fn func()) { t.onClear = fn }
func (t *Tray) OnQuit(fn func())  { t.onQuit = fn }

func (t *Tray) Run(onReady func()) {
	if statusNotifierAvailable() {
		systray.Run(func() {
			t.setup()
			if onReady != nil {
				onReady()
			}
		}, func() {
			if t.onQuit != nil {
				t.onQuit()
			}
		})
		return
	}

	stop, err := startXEmbed(func() {
		if t.onShow != nil {
			t.onShow()
		}
	})
	if err == nil {
		log.Printf("tray: no StatusNotifier watcher; using the XEmbed system tray (click the icon to show history)")
		if onReady != nil {
			onReady()
		}
		<-t.quit
		stop()
		if t.onQuit != nil {
			t.onQuit()
		}
		return
	}

	log.Printf("tray: no StatusNotifier watcher and no XEmbed tray (%v). History stays available with %s or `linboard toggle`.", err, config.HotkeyLabel)
	if onReady != nil {
		onReady()
	}
	<-t.quit
	if t.onQuit != nil {
		t.onQuit()
	}
}

func (t *Tray) setup() {
	systray.SetIcon(assets.TrayPNG())
	systray.SetTitle("LinBoard")
	systray.SetTooltip(config.AppName)

	mShow := systray.AddMenuItem(
		"Show History ("+config.HotkeyLabel+")",
		"Open clipboard history",
	)
	systray.AddSeparator()

	t.countItem = systray.AddMenuItem("0 items in history", "")
	t.countItem.Disable()

	systray.AddSeparator()
	mClear := systray.AddMenuItem("Clear History", "Remove all unpinned items")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit LinBoard", "Exit the application")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if t.onShow != nil {
					t.onShow()
				}
			case <-mClear.ClickedCh:
				if t.onClear != nil {
					t.onClear()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (t *Tray) SetCount(n int) {
	if t.countItem != nil {
		t.countItem.SetTitle(formatCount(n))
	}
}

func (t *Tray) Quit() {
	t.quitOnce.Do(func() { close(t.quit) })
	systray.Quit()
}

func formatCount(n int) string {
	switch n {
	case 0:
		return "No items in history"
	case 1:
		return "1 item in history"
	default:
		return fmt.Sprintf("%d items in history", n)
	}
}
