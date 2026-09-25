package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/upsert"
)

// upsertVerifyView reports whether an UPSERT_ENTITY action actually wrote its
// entity.
//
// There is no run watcher here on purpose. Port performs these actions itself
// and discards the run record immediately, so the id it just handed us 404s
// from that moment on whether the upsert succeeded or was rejected. A status
// poll would show a permanent error and a log tail a permanently empty list,
// both of which would look like failures regardless of the outcome. The
// entity is the only evidence there is.
type upsertVerifyView struct {
	app  *App
	spec FormSpec
	req  upsert.Request
	// pre is the attempt this view is reporting on, kept so a retry can
	// rebuild the form from it rather than from the bare schema.
	pre runPrefill

	view *tview.TextView
	ref  refresher

	entity     *client.Entity
	failure    error
	unresolved []string
	done       bool
}

func newUpsertVerifyView(a *App, spec FormSpec, req upsert.Request, pre runPrefill) *upsertVerifyView {
	v := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	v.SetTitle(" upsert verification ").SetBorder(true).SetBorderColor(colorBorder)

	u := &upsertVerifyView{app: a, spec: spec, req: req, pre: pre, view: v}
	u.ref.app = a.app
	u.render()
	return u
}

func (v *upsertVerifyView) ID() string                 { return "upsert/" + v.req.ActionID }
func (v *upsertVerifyView) Title() string              { return "verify(" + v.req.ActionID + ")" }
func (v *upsertVerifyView) Primitive() tview.Primitive { return v.view }
func (v *upsertVerifyView) Stop()                      { v.ref.stop() }

func (v *upsertVerifyView) Ops() []Op {
	var ops []Op
	if v.entity != nil {
		ops = append(ops, Op{Rune: 'd', Name: "Describe entity", Run: func(a *App, _ []Row) error {
			a.push(newDescribeView(a, v.spec.Blueprint+"/"+v.entity.Entity.Identifier,
				v.entity.Entity.Identifier, v.entity.Entity))
			return nil
		}})
	}
	if v.done && v.entity == nil {
		// Reopening the form with the previous answers is the whole point: the
		// usual fix is one input that resolved empty, and retyping the rest is
		// pure friction.
		ops = append(ops, Op{Rune: 'r', Name: "Retry with these inputs", Run: func(a *App, _ []Row) error {
			return v.retry(a)
		}})
	}
	return ops
}

// Start runs the verification once. It is a single bounded poll, not a
// refresh loop: the question "did the entity appear" has one answer.
func (v *upsertVerifyView) Start(ctx context.Context) {
	if v.done {
		return
	}
	v.ref.start(ctx, verifyOnce,
		func(ctx context.Context) (any, error) {
			// upsert.Verify polls for up to 10s at 250ms, because the write is
			// not synchronous with the response that triggered it.
			return upsert.Verify(ctx, v.app.client, v.req)
		},
		func(data any, err error) {
			v.done = true
			v.ref.stop()
			if err != nil {
				v.failure = err
				v.collectDiagnosis(ctx)
			} else if entity, _ := data.(*client.Entity); entity != nil {
				v.entity = entity
				v.app.flash.show(flashInfo, "entity %s created", entity.Entity.Identifier)
			}
			v.render()
			v.app.drawHeader()
		})
}

// verifyOnce is an interval long enough that the loop never fires a second
// time; Start stops the refresher as soon as the first result lands. Reusing
// the refresher rather than a bare goroutine keeps the cancellation and
// UI-goroutine discipline that makes background work safe here.
const verifyOnce = 24 * time.Hour

// collectDiagnosis asks which required properties came out empty. Verify
// already embeds this in its error text, but the list is wanted separately so
// the retry can point at the offending input.
func (v *upsertVerifyView) collectDiagnosis(ctx context.Context) {
	action, err := v.app.client.GetAction(ctx, v.req.ActionID)
	if err != nil {
		return
	}
	v.unresolved = upsert.UnresolvedRequired(ctx, v.app.client, action, v.req.RunProps)
}

func (v *upsertVerifyView) retry(a *App) error {
	action, err := a.client.GetAction(a.ctx, v.req.ActionID)
	if err != nil {
		return fmt.Errorf("cannot reopen the form: %w", err)
	}
	// The previous answers and target are carried over, so only the input
	// that resolved empty needs attention.
	form := newRunFormView(a, *action, v.pre)

	a.pop() // leave the verification behind; it is about the previous attempt
	// And pop the form under it too. submit pushed this view without popping
	// the form that produced it, so that form's page is still registered —
	// and push skips AddPage when a page of the same ID already exists, which
	// would leave the old primitive on screen under the new view.
	a.pop()
	a.push(form)
	a.flash.show(flashInfo, "previous inputs carried over — fill the unresolved one")
	return nil
}

func (v *upsertVerifyView) render() {
	var b strings.Builder
	field := func(name, value string) { b.WriteString(fieldLine(name, value, 12) + "\n") }

	field("Action", v.req.ActionID)
	field("Backend", "UPSERT_ENTITY")
	field("Run ID", v.req.RunID+
		"  [dimgray](Port keeps no record for these — this id will never resolve, by design)[-]")
	if v.spec.Blueprint != "" {
		field("Target", v.spec.Blueprint+"/"+v.req.Identifier)
	}
	b.WriteString("\n")

	switch {
	case !v.done:
		b.WriteString("[orange]⟳ polling for the entity (10s budget, 250ms interval)[-]\n")

	case v.entity != nil:
		fmt.Fprintf(&b, "[palegreen]✓ entity %s exists — the upsert landed.[-]\n\n",
			v.entity.Entity.Identifier)
		b.WriteString("[dimgray]press d to inspect it[-]\n")

	default:
		b.WriteString("[indianred::b]✗ the entity was not created.[-]\n\n")
		if len(v.unresolved) > 0 {
			// This is the diagnosis worth having: a required blueprint
			// property whose mapped input resolved to nothing. The usual
			// culprit is a hidden input defaulted from a jqQuery over the
			// calling user, which is empty under client credentials.
			b.WriteString("[orange]required properties that did not resolve:[-]\n")
			for _, u := range v.unresolved {
				fmt.Fprintf(&b, "  • %s\n", u)
			}
			b.WriteString("\n[dimgray]press r to reopen the form with these inputs and fill the empty one[-]\n")
		} else if v.failure != nil {
			fmt.Fprintf(&b, "%s\n", v.failure)
		}
	}

	v.view.SetText(b.String())
}
