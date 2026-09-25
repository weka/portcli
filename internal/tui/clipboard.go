package tui

import (
	"encoding/base64"
	"fmt"
	"os"
)

// osc52MaxBytes caps what we try to send. Terminals impose their own limits on
// escape-sequence length and silently truncate or drop past them, so a huge
// payload is refused with an explanation rather than half-copied.
const osc52MaxBytes = 64 * 1024

// osc52 copies text to the system clipboard using the OSC 52 escape sequence.
//
// The terminal emulator performs the copy, which is what makes this work over
// ssh — a clipboard library linked into this process would be writing to the
// clipboard of whichever machine portcli runs on, not the one being looked at.
// Not every terminal honours it, and none of them acknowledge it, so success
// here means "sent", not "pasted".
func osc52(text string) error {
	if len(text) > osc52MaxBytes {
		return fmt.Errorf("too large to copy via the terminal (%d bytes, limit %d)", len(text), osc52MaxBytes)
	}
	// Write straight to the tty: stdout may be redirected, and the sequence
	// has to reach the terminal rather than a file.
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("no terminal to copy through: %w", err)
	}
	defer tty.Close()

	_, err = fmt.Fprintf(tty, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
	return err
}
