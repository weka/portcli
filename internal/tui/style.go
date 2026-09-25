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

// The palette follows k9s: amber for the labels that name things, cyan for
// keys and identifiers, and a solid cyan bar for the selected row.
const (
	colorLabel    = tcell.ColorOrange      // "Base URL:", "Auth:" — the left column
	colorValue    = tcell.ColorWhite       // what those labels point at
	colorKey      = tcell.ColorDeepSkyBlue // <d>, <ctrl-r> in the legend
	colorHint     = tcell.ColorSilver      // what a key does
	colorTitle    = tcell.ColorWhite       // column headings, box titles
	colorAccent   = tcell.ColorAqua        // identifiers, the live part of a title
	colorBorder   = tcell.ColorGray
	colorLogo     = tcell.ColorOrange
	colorSelected = tcell.ColorAqua // selection bar; text on it is black
	colorDimmed   = tcell.ColorGray // rows in a terminal state

	colorInfo  = tcell.ColorPaleGreen
	colorWarn  = tcell.ColorOrange
	colorError = tcell.ColorIndianRed
)

// Markup tag names for the same colours, for the text views that take tags
// rather than styles.
const (
	tagLabel  = "orange"
	tagValue  = "white"
	tagKey    = "deepskyblue"
	tagHint   = "silver"
	tagAccent = "aqua"
	tagDim    = "gray"
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
