#!/bin/bash
#
# sign_macos.sh — sign and notarize macOS (Darwin) binaries.
#
# Required environment variables:
#
#   MACOS_CERTIFICATE       Base64-encoded .p12 Developer ID certificate
#   MACOS_CERTIFICATE_PWD   Password for the .p12 file
#   KEYCHAIN_PASSWORD       Arbitrary password for the temporary build keychain
#   CERT_IDENTITY           Signing identity (e.g. "Developer ID Application: Your Org (TEAMID)")
#   APPLE_ID                Apple ID email for notarytool
#   TEAM_ID                 Apple Developer Team ID
#   APP_PASSWORD            App-specific password for notarytool
#   ARTIFACTS_DIR           Directory containing the unsigned Darwin binaries

set -euo pipefail

: "${MACOS_CERTIFICATE:?must be set}"
: "${MACOS_CERTIFICATE_PWD:?must be set}"
: "${KEYCHAIN_PASSWORD:?must be set}"
: "${CERT_IDENTITY:?must be set}"
: "${APPLE_ID:?must be set}"
: "${TEAM_ID:?must be set}"
: "${APP_PASSWORD:?must be set}"
: "${ARTIFACTS_DIR:?must be set}"

KEYCHAIN="build.keychain"

cleanup() {
    security delete-keychain "$KEYCHAIN" 2>/dev/null || true
    rm -f certificate.p12
}
trap cleanup EXIT

# --- Set up signing keychain ---
echo "==> Importing certificate into temporary keychain"
echo "$MACOS_CERTIFICATE" | base64 --decode >certificate.p12
security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
security default-keychain -s "$KEYCHAIN"
security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
security import certificate.p12 -k "$KEYCHAIN" -P "$MACOS_CERTIFICATE_PWD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$KEYCHAIN_PASSWORD" "$KEYCHAIN"

# --- Process each Darwin binary ---
for binary in "$ARTIFACTS_DIR"/portcli_*_darwin_*; do
    [ -f "$binary" ] || continue
    echo "==> Processing $binary"

    # Sign with hardened runtime (required for notarization)
    echo "    Signing..."
    /usr/bin/codesign --force --options runtime --timestamp -s "$CERT_IDENTITY" "$binary"

    # Notarize — zip is required for notarytool submission
    echo "    Notarizing..."
    zipdir=$(mktemp -d)
    zip -j "$zipdir/portcli.zip" "$binary"
    xcrun notarytool submit "$zipdir/portcli.zip" \
        --apple-id "$APPLE_ID" \
        --team-id "$TEAM_ID" \
        --password "$APP_PASSWORD" \
        --wait
    rm -rf "$zipdir"

    echo "    Done: $binary"
done

echo "==> All Darwin binaries signed and notarized"
