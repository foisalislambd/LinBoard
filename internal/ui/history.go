package ui

import (
	"image/color"
	"log"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/foisal/linboard/internal/assets"
	"github.com/foisal/linboard/internal/clipboard"
	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
	"github.com/foisal/linboard/internal/store"
)

// pasteRow wraps a list row and pastes its clip on mouse click (CopyQ-style).
type pasteRow struct {
	widget.BaseWidget
	row     fyne.CanvasObject
	onPaste func()
}

func newPasteRow(row fyne.CanvasObject) *pasteRow {
	p := &pasteRow{row: row}
	p.ExtendBaseWidget(p)
	return p
}

func (p *pasteRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.row)
}

func (p *pasteRow) Tapped(*fyne.PointEvent) {
	if p.onPaste != nil {
		p.onPaste()
	}
}

func (p *pasteRow) TappedSecondary(*fyne.PointEvent) {}

type HistoryWindow struct {
	app      fyne.App
	win      fyne.Window
	store    *store.Store
	cfg      *config.Config
	list     *widget.List
	clips    []store.Clip
	selected int
	search   *widget.Entry
	mu       sync.Mutex
	visible  bool
	lastToggle time.Time
}

func NewHistoryWindow(app fyne.App, s *store.Store, cfg *config.Config) *HistoryWindow {
	h := &HistoryWindow{
		app:   app,
		store: s,
		cfg:   cfg,
	}
	h.build()
	return h
}

func (h *HistoryWindow) build() {
	h.win = h.app.NewWindow(config.AppName)
	h.win.SetIcon(assets.Fyne())
	h.win.SetFixedSize(true)
	h.win.Resize(fyne.NewSize(480, 420))
	h.win.SetCloseIntercept(func() {
		h.Hide()
	})

	h.search = widget.NewEntry()
	h.search.SetPlaceHolder("Search clipboard history…")
	h.search.OnChanged = func(_ string) {
		h.refreshList()
	}

	h.list = widget.NewList(
		func() int {
			h.mu.Lock()
			defer h.mu.Unlock()
			return len(h.clips)
		},
		func() fyne.CanvasObject {
			pinIcon := widget.NewIcon(theme.MediaRecordIcon())
			preview := widget.NewLabel("")
			preview.Wrapping = fyne.TextTruncate
			copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), nil)
			copyBtn.Importance = widget.LowImportance
			timeLabel := widget.NewLabel("")
			timeLabel.TextStyle = fyne.TextStyle{Italic: true}
			typeBadge := widget.NewLabel("")
			typeBadge.TextStyle = fyne.TextStyle{Bold: true}
			border := container.NewBorder(
				nil, nil,
				container.NewHBox(pinIcon, typeBadge),
				container.NewHBox(copyBtn, timeLabel),
				preview,
			)
			return newPasteRow(border)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			h.mu.Lock()
			if id < 0 || id >= len(h.clips) {
				h.mu.Unlock()
				return
			}
			clip := h.clips[id]
			h.mu.Unlock()

			row := obj.(*pasteRow)
			itemID := int(id)
			row.onPaste = func() {
				h.list.Select(widget.ListItemID(itemID))
				h.pasteClipAt(itemID)
			}

			// NewBorder order: center, left, right (see fyne container.NewBorder)
			border := row.row.(*fyne.Container)
			preview := border.Objects[0].(*widget.Label)
			left := border.Objects[1].(*fyne.Container)
			right := border.Objects[2].(*fyne.Container)
			copyBtn := right.Objects[0].(*widget.Button)
			timeLabel := right.Objects[1].(*widget.Label)
			pinIcon := left.Objects[0].(*widget.Icon)
			typeBadge := left.Objects[1].(*widget.Label)

			if clip.Pinned {
				pinIcon.SetResource(theme.MediaRecordIcon())
				pinIcon.Show()
			} else {
				pinIcon.Hide()
			}

			switch clip.ContentType {
			case store.TypeURL:
				typeBadge.SetText("URL")
			case store.TypeImage:
				typeBadge.SetText("IMG")
			default:
				typeBadge.SetText("TXT")
			}

			preview.SetText(clip.Preview)
			timeLabel.SetText(store.FormatTime(clip.CreatedAt))
			c := clip
			copyBtn.OnTapped = func() {
				if err := clipboard.CopyClip(&c); err != nil {
					log.Printf("copy failed: %v", err)
				}
			}
		},
	)

	h.list.OnSelected = func(id widget.ListItemID) {
		h.mu.Lock()
		h.selected = int(id)
		h.mu.Unlock()
	}
	h.list.OnUnselected = func(_ widget.ListItemID) {}

	help := widget.NewLabel("Click row to paste  •  ↑↓ Navigate  •  Enter Paste  •  Del Remove  •  P Pin  •  Esc Close")
	help.TextStyle = fyne.TextStyle{Italic: true}
	help.Alignment = fyne.TextAlignCenter

	content := container.NewBorder(
		container.NewVBox(
			h.search,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			help,
		),
		nil, nil,
		h.list,
	)

	bg := canvas.NewRectangle(historyBackground(h.app))
	bg.CornerRadius = 8
	h.win.SetContent(container.NewStack(bg, container.NewPadded(content)))

	h.win.Canvas().SetOnTypedKey(h.handleKey)
	h.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyEscape,
		Modifier: 0,
	}, func(_ fyne.Shortcut) {
		h.Hide()
	})
}

func (h *HistoryWindow) handleKey(ev *fyne.KeyEvent) {
	// Let the search field handle printable text input.
	if h.win.Canvas().Focused() == h.search && ev.Name == fyne.KeyP {
		return
	}

	switch ev.Name {
	case fyne.KeyEscape:
		h.Hide()
	case fyne.KeyReturn, fyne.KeyEnter:
		h.pasteSelected()
	case fyne.KeyUp:
		h.mu.Lock()
		sel := h.selected
		h.mu.Unlock()
		if sel > 0 {
			h.list.Select(sel - 1)
		}
	case fyne.KeyDown:
		h.mu.Lock()
		sel := h.selected
		count := len(h.clips)
		h.mu.Unlock()
		if sel < count-1 {
			h.list.Select(sel + 1)
		}
	case fyne.KeyDelete:
		h.deleteSelected()
	case fyne.KeyP:
		h.pinSelected()
	}
}

func (h *HistoryWindow) RefreshIfVisible() {
	h.mu.Lock()
	visible := h.visible
	h.mu.Unlock()
	if visible {
		h.refreshList()
	}
}

func (h *HistoryWindow) refreshList() {
	search := strings.TrimSpace(h.search.Text)
	limit := h.cfg.MaxHistory
	if limit <= 0 {
		limit = 100
	}
	clips, err := h.store.List(search, limit)
	if err != nil {
		log.Printf("list clips: %v", err)
		return
	}
	h.mu.Lock()
	h.clips = clips
	if h.selected >= len(h.clips) {
		h.selected = 0
	}
	h.mu.Unlock()
	h.list.Refresh()
	if len(h.clips) > 0 {
		h.list.Select(h.selected)
	}
}

func (h *HistoryWindow) pasteSelected() {
	h.mu.Lock()
	sel := h.selected
	h.mu.Unlock()
	h.pasteClipAt(sel)
}

func (h *HistoryWindow) pasteClipAt(id int) {
	h.mu.Lock()
	if id < 0 || id >= len(h.clips) {
		h.mu.Unlock()
		return
	}
	clip := h.clips[id]
	h.mu.Unlock()

	if err := clipboard.CopyClip(&clip); err != nil {
		log.Printf("copy failed: %v", err)
		return
	}
	h.Hide()
	if !h.cfg.PasteOnSelect {
		return
	}
	go func() {
		if err := clipboard.PasteToTarget(); err != nil {
			log.Printf("paste failed: %v", err)
		}
	}()
}

func (h *HistoryWindow) deleteSelected() {
	h.mu.Lock()
	if h.selected < 0 || h.selected >= len(h.clips) {
		h.mu.Unlock()
		return
	}
	id := h.clips[h.selected].ID
	h.mu.Unlock()
	if err := h.store.Delete(id); err != nil {
		log.Printf("delete clip: %v", err)
	}
	h.refreshList()
}

func (h *HistoryWindow) pinSelected() {
	h.mu.Lock()
	if h.selected < 0 || h.selected >= len(h.clips) {
		h.mu.Unlock()
		return
	}
	id := h.clips[h.selected].ID
	h.mu.Unlock()
	if err := h.store.TogglePin(id); err != nil {
		log.Printf("pin clip: %v", err)
	}
	h.refreshList()
}

func (h *HistoryWindow) Toggle() {
	h.mu.Lock()
	if time.Since(h.lastToggle) < 300*time.Millisecond {
		h.mu.Unlock()
		return
	}
	h.lastToggle = time.Now()
	visible := h.visible
	h.mu.Unlock()
	if visible {
		h.Hide()
	} else {
		platform.CapturePasteTarget()
		h.Show()
	}
}

func (h *HistoryWindow) Show() {
	h.search.SetText("")
	h.refreshList()
	h.win.CenterOnScreen()
	h.win.Show()
	h.win.RequestFocus()
	h.mu.Lock()
	h.visible = true
	count := len(h.clips)
	h.mu.Unlock()
	if count > 0 {
		h.list.Select(0)
	}
}

func (h *HistoryWindow) Hide() {
	h.win.Hide()
	h.mu.Lock()
	h.visible = false
	h.mu.Unlock()
}

func historyBackground(app fyne.App) color.Color {
	th := app.Settings().Theme()
	variant := app.Settings().ThemeVariant()
	c := th.Color(theme.ColorNameOverlayBackground, variant)
	if nrgba, ok := c.(color.NRGBA); ok {
		nrgba.A = 250
		return nrgba
	}
	return c
}
