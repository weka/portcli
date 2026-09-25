package tui

import "github.com/gdamore/tcell/v2"

// The palette is deliberately small and sticks to the terminal's own colours
// where it can, so it inherits whatever scheme the user already has.
const (
	colorHeaderKey   = tcell.ColorDarkCyan
	colorHeaderValue = tcell.ColorWhite
	colorTitle       = tcell.ColorDodgerBlue
	colorBorder      = tcell.ColorGray
	colorHint        = tcell.ColorDimGray
	colorSelected    = tcell.ColorDarkSlateGray

	colorInfo  = tcell.ColorPaleGreen
	colorWarn  = tcell.ColorOrange
	colorError = tcell.ColorIndianRed
)

// statusColor maps a Port run or entity status onto a colour. Anything
// unrecognised stays default rather than being guessed at, since blueprints
// define their own status vocabularies.
func statusColor(status string) tcell.Color {
	switch status {
	case "SUCCESS", "Ready", "OK", "READY", "Passed", "PASS":
		return colorInfo
	case "IN_PROGRESS", "Creating", "Destroying", "Provisioning", "WAITING_FOR_APPROVAL":
		return colorWarn
	case "FAILURE", "Failed", "FAILED", "Error", "Fail":
		return colorError
	default:
		return tcell.ColorDefault
	}
}
