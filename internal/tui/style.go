package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// tview draws a doubled border on whichever box has focus. k9s frames
// everything in the same single line, and a border that thickens as focus
// moves is noise rather than information.
func init() {
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
}

// The palette is the k9s default skin, with each constant named after the
// part of the frame k9s applies it to, so the two read the same side by side.
const (
	colorLabel    = tcell.ColorOrange     // info: "Base URL:", "Auth:"
	colorValue    = tcell.ColorWhite      // info: what those labels point at
	colorKey      = tcell.ColorDodgerBlue // menu: <d>, <ctrl-r>
	colorHint     = tcell.ColorSilver     // menu: what a key does
	colorTitle    = tcell.ColorWhite      // table column headings
	colorAccent   = tcell.ColorAqua       // frame titles, identifiers
	colorBorder   = tcell.ColorDodgerBlue // every box in the frame
	colorLogo     = tcell.ColorOrange
	colorSelected = tcell.ColorAqua // cursor bar; text on it is black
	colorDimmed   = tcell.ColorGray // rows in a terminal state

	// Pieces of a frame title. k9s gives the row count and the active filter
	// their own colours so both read at a glance out of the aqua title.
	colorCounter   = tcell.ColorPapayaWhip
	colorFilter    = tcell.ColorSeaGreen
	colorHighlight = tcell.ColorFuchsia

	// The prompt's border says which prompt it is. k9s colours ":" and "/"
	// differently so the mode is visible without reading the label — and
	// filter mode borrows the same green the filter wears in the title.
	colorCommand = tcell.ColorAqua
	// Fill for the completion list, which floats over the table rather than
	// displacing it, so it has to be visibly not the background.
	colorPanel = tcell.ColorDarkSlateGray

	colorInfo  = tcell.ColorPaleGreen
	colorWarn  = tcell.ColorOrange
	colorError = tcell.ColorOrangeRed
)

// Markup tag names for the same colours, for the text views that take tags
// rather than styles. Derived rather than restated: Color.Name round-trips
// through tcell.GetColor, which is the lookup tview's tag parser performs, so
// a tag cannot drift out of step with the constant it names.
var (
	tagLabel     = colorLabel.Name()
	tagValue     = colorValue.Name()
	tagKey       = colorKey.Name()
	tagHint      = colorHint.Name()
	tagAccent    = colorAccent.Name()
	tagDim       = colorDimmed.Name()
	tagCounter   = colorCounter.Name()
	tagFilter    = colorFilter.Name()
	tagHighlight = colorHighlight.Name()
	tagInfo      = colorInfo.Name()
	tagWarn      = colorWarn.Name()
	tagError     = colorError.Name()
)

// logoLines is the banner in the top right, in the spirit of the k9s one.
// Padded to equal width: the banner is right-aligned, so ragged lines would
// stagger it.
var logoLines = []string{
	` ___   __  ___ _____ `,
	`| _ \ /  \| _ \_   _| `,
	`|  _/| () |   / | |   `,
	`|_|   \__/|_|_\ |_|   `,
	`     p o r t c l i    `,
}

// inertStatuses are states where the row is spent — the thing is gone, or was
// abandoned — and k9s greys those out so live rows stand out.
//
// Deliberately narrow. SUCCESS and FAILURE are outcomes worth reading, not
// noise: listing them here dimmed entire tables to grey, since almost every
// run ends SUCCESS. Those get their colour from statusColor instead.
var inertStatuses = map[string]bool{
	"Completed": true, "CANCELLED": true, "Cancelled": true,
	"Destroyed": true, "DELETED": true, "Deleted": true,
}

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
