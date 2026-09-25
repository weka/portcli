#!/usr/bin/env bash
#
# Check that a change did not alter CLI behaviour.
#
# Builds <ref> (default HEAD) in a throwaway git worktree, builds the working
# tree, then runs both binaries over the invocations below, diffing stdout,
# stderr and exit code.
#
# The two binaries run back-to-back per case on purpose. Entities and action
# runs change while you work, so comparing against output captured minutes or
# hours earlier reports live tenant drift as a regression. Requires working
# credentials; cases touching the catalog are skipped without them.
#
# Usage: scripts/compare_cli.sh [ref]
set -uo pipefail

REF="${1:-HEAD}"
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
WT="$TMP/ref-worktree"
trap 'cd "$REPO" && git worktree remove --force "$WT" 2>/dev/null; rm -rf "$TMP"' EXIT

fail=0
p() { printf '%-26s %s\n' "$1" "$2"; }

cd "$REPO" || exit 1

# A blueprint to exercise the entity commands against. Override when the
# default is absent from your tenant.
BP="${PORTCLI_TEST_BLUEPRINT:-lab-server}"
ACT="${PORTCLI_TEST_ACTION:-testservice-deployment-client_clients_deployment}"

git worktree add --quiet --detach "$WT" "$REF" || { echo "cannot create a worktree at $REF"; exit 1; }

echo "== building =="
( cd "$WT" && go build -o "$TMP/pc-ref" . ) || { echo "ref build failed"; exit 1; }
go build -o "$TMP/pc-new" . || { echo "working-tree build failed"; exit 1; }
p "ref ($REF)" "built"
p "working tree" "built"

cases=(
  "help|--help"
  "version|--version"
  "bare|"
  "unknown-command|entty"
  "blueprint-list|blueprint list"
  "blueprint-filter|blueprint list --filter ^lab"
  "blueprint-get|blueprint get $BP"
  "entity-list|entity get $BP"
  "entity-columns|entity get $BP -c status"
  "entity-filter|entity get $BP -f status=OK"
  "entity-missing|entity get $BP no-such-entity-xyz"
  "entity-bad-filter|entity get $BP -f nofield"
  "runs-list|action list --limit 5"
  "runs-json|action list --json --limit 5"
  "runs-status|action list --limit 5 --status SUCCESS"
  "action-get|action get $ACT"
  "action-get-json|action get $ACT --json"
  "run-missing|action status r_does_not_exist_xyz"
  "logs-missing|action logs r_does_not_exist_xyz"
)

echo
echo "== $REF vs working tree =="
for c in "${cases[@]}"; do
  name="${c%%|*}"; args="${c#*|}"
  argv=(); [ -n "$args" ] && read -ra argv <<< "$args"

  "$TMP/pc-ref" "${argv[@]+"${argv[@]}"}" >"$TMP/r.out" 2>"$TMP/r.err"; rc=$?
  "$TMP/pc-new" "${argv[@]+"${argv[@]}"}" >"$TMP/n.out" 2>"$TMP/n.err"; nc=$?

  d=""
  cmp -s "$TMP/r.out" "$TMP/n.out" || d="$d stdout"
  cmp -s "$TMP/r.err" "$TMP/n.err" || d="$d stderr"
  [ "$rc" = "$nc" ] || d="$d exit($rc->$nc)"

  if [ -z "$d" ]; then
    p "$name" "same"
  else
    p "$name" "DIFFERS:$d"
    fail=1
    diff <(head -c 1200 "$TMP/r.out") <(head -c 1200 "$TMP/n.out") | head -10
    diff <(head -c 600 "$TMP/r.err") <(head -c 600 "$TMP/n.err") | head -6
  fi
done

echo
if [ $fail -eq 0 ]; then
  echo "RESULT: behaviour identical to $REF"
else
  echo "RESULT: differences above — investigate before committing"
fi
exit $fail
