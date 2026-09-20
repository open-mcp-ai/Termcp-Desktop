package main

// SystemFonts returns installed font family names for the appearance settings.
// Platform implementations keep operating-system specific discovery out of the UI.
func (a *App) SystemFonts() []string {
	fonts := systemFontFamilies()
	if len(fonts) == 0 {
		return []string{"system-ui", "Arial", "Helvetica", "Times New Roman", "Courier New"}
	}
	return fonts
}
