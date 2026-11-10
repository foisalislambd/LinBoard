//go:build !linux

package clipboard

func UinputWritable() bool { return false }

func InInputGroupSession() bool    { return false }
func InInputGroupConfigured() bool { return false }
func SessionNeedsRelogin() bool    { return false }
func PasteSessionHint() string     { return PasteSetupHint() }
