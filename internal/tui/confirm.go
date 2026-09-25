package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// confirm asks before running a destructive operation.
//
// There is no --yes equivalent here on purpose: the CLI has one because a
// script cannot answer a prompt, whereas a person driving a TUI always can.
// Focus starts on Cancel so a reflexive Enter is safe.
func (a *App) confirm(op Op, rows []Row) {
	modal := tview.NewModal().
		SetText(confirmText(op, rows)).
		AddButtons([]string{"Cancel", op.Name}).
		SetDoneFunc(func(index int, _ string) {
			a.closeOverlay("confirm")
			if index != 1 {
				a.flash.show(flashInfo, "cancelled")
				return
			}
			if err := op.Run(a, rows); err != nil {
				a.showError(err)
			}
		})
	modal.SetFocus(0)
	a.pages.AddPage("confirm", modal, true, true)
	a.app.SetFocus(modal)
}

// confirmText names what is about to happen, listing a few identifiers so a
// bulk operation is not just a count to take on trust.
func confirmText(op Op, rows []Row) string {
	if len(rows) == 1 {
		return fmt.Sprintf("%s %s?\n\nThis cannot be undone.", op.Name, rows[0].ID)
	}

	const preview = 5
	names := make([]string, 0, preview)
	for _, r := range rows[:min(preview, len(rows))] {
		names = append(names, r.ID)
	}
	list := strings.Join(names, "\n")
	if len(rows) > preview {
		list += fmt.Sprintf("\n…and %d more", len(rows)-preview)
	}
	return fmt.Sprintf("%s %d entities?\n\n%s\n\nThis cannot be undone.", op.Name, len(rows), list)
}
