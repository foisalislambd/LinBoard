package app

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// variantTheme forces a light or dark palette while keeping the default fonts and icons.
type variantTheme struct {
	base    fyne.Theme
	variant fyne.ThemeVariant
}

func forcedVariantTheme(variant fyne.ThemeVariant) fyne.Theme {
	return variantTheme{base: theme.DefaultTheme(), variant: variant}
}

func (t variantTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.base.Color(name, t.variant)
}

func (t variantTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t variantTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t variantTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}
