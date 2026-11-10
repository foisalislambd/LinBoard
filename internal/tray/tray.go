package tray

import (
	"fmt"

	"fyne.io/systray"

	"github.com/foisal/linboard/internal/assets"
	"github.com/foisal/linboard/internal/config"
)

type Tray struct {
	onShow    func()
	onClear   func()
	onQuit    func()
	countItem *systray.MenuItem
}

func New() *Tray {
	return &Tray{}
}

func (t *Tray) OnShow(fn func())  { t.onShow = fn }
func (t *Tray) OnClear(fn func()) { t.onClear = fn }
func (t *Tray) OnQuit(fn func())  { t.onQuit = fn }

func (t *Tray) Run(onReady func()) {
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
