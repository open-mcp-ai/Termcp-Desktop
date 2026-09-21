// Package fonts discovers installed font family names for the appearance
// settings. Platform specific discovery lives in families_*.go.
package fonts

var fallback = []string{"system-ui", "Arial", "Helvetica", "Times New Roman", "Courier New"}

// Families returns the installed font families, or a small fallback list when
// discovery is unavailable on the current platform.
func Families() []string {
	fonts := families()
	if len(fonts) == 0 {
		return append([]string(nil), fallback...)
	}
	return fonts
}
