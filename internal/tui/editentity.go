package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Diff returns the entries of next that differ from prev.
//
// UpdateEntityProperties is a PATCH, so sending only what changed is both
// smaller and safer: echoing back every property risks writing calculated or
// mirrored values that the API returned to us but does not accept as input.
//
// A key removed from next is not treated as a deletion. PATCH-with-null means
// different things per property type, so removing a property is deliberately
// not something this offers.
func Diff(prev, next map[string]any) map[string]any {
	changed := map[string]any{}
	for key, val := range next {
		if old, existed := prev[key]; !existed || !reflect.DeepEqual(old, val) {
			changed[key] = val
		}
	}
	return changed
}

// DiffSummary names the changed keys, sorted, for a flash message.
func DiffSummary(changed map[string]any) string {
	names := make([]string, 0, len(changed))
	for key := range changed {
		names = append(names, key)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// editorCommand returns the editor to use, honouring the conventional
// variables before falling back to something that exists everywhere.
func editorCommand() []string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			// Allow "code --wait" and friends.
			return strings.Fields(v)
		}
	}
	return []string{"vi"}
}

// editJSONInEditor suspends the UI, opens doc in the user's editor, and
// returns what they saved.
//
// Suspending is how k9s does this, and it is the right trade: a real editor
// handles nested values, large pastes and undo, none of which a form built
// inside the TUI would. tview restores the screen itself on return.
//
// An invalid save is handed back to the editor with the parse error prepended
// as a comment rather than discarded — losing someone's edit because of a
// stray comma would be worse than asking again.
func (a *App) editJSONInEditor(name string, doc map[string]any) (map[string]any, bool, error) {
	pretty, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("cannot render %s as JSON: %w", name, err)
	}

	dir, err := os.MkdirTemp("", "portcli-edit-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)

	// The filename is what the editor shows and what its syntax highlighting
	// keys off, so it names the entity and ends in .json.
	path := filepath.Join(dir, sanitizeFilename(name)+".json")
	contents := append(pretty, '\n')

	for attempt := 0; ; attempt++ {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			return nil, false, err
		}

		var runErr error
		suspended := a.app.Suspend(func() {
			argv := editorCommand()
			cmd := exec.Command(argv[0], append(argv[1:], path)...) // #nosec G204 -- the editor is the user's own choice
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			runErr = cmd.Run()
		})
		if !suspended {
			return nil, false, fmt.Errorf("cannot suspend the UI to open an editor")
		}
		if runErr != nil {
			return nil, false, fmt.Errorf("%s: %w", strings.Join(editorCommand(), " "), runErr)
		}

		saved, err := os.ReadFile(path)
		if err != nil {
			return nil, false, err
		}

		body := stripComments(saved)
		if strings.TrimSpace(string(body)) == "" {
			// An emptied buffer is the conventional way to abort.
			return nil, false, nil
		}

		var edited map[string]any
		if err := json.Unmarshal(body, &edited); err == nil {
			return edited, true, nil
		} else if attempt >= 2 {
			return nil, false, fmt.Errorf("still not valid JSON after %d attempts: %w", attempt+1, err)
		} else {
			contents = append([]byte(fmt.Sprintf("// not valid JSON: %v\n// fix the error below, or empty this file to cancel.\n", err)), body...)
		}
	}
}

// stripComments removes the "//" lines this file may have prepended, so a
// retry does not accumulate them and they never reach the JSON parser.
func stripComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n"))
}

// sanitizeFilename keeps an entity identifier usable as a filename.
func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, s)
}
