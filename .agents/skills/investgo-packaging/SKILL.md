---
name: investgo-packaging
description: Package InvestGo desktop releases and review platform-specific release gaps. Use when requests involve macOS app bundle or DMG packaging, release artifacts, signing, notarization, packaging script changes, or future Windows x64 release planning.
---

# InvestGo Packaging

## Workflow

1. Read `references/packaging-matrix.md` before running or editing packaging steps.
2. Package only after the build is stable. The packaging script depends on the build script and inherits its assumptions.
3. Use `./scripts/package-darwin-aarch64.sh` for Apple Silicon macOS packages and `./scripts/package-darwin-x86_64.sh` for Intel macOS packages. Add `--dev` only when a debug-capable packaged app is required.
4. Respect packaging environment variables such as `VERSION`, `APP_ID`, `APPLE_SIGN_IDENTITY`, and `NOTARYTOOL_PROFILE`. Do not hardcode release metadata inside the script.
5. Keep packaging concerns separate from compilation concerns where possible. Shared preparation is fine; platform-specific bundling should stay explicit.
6. For future Windows x64 support, design a separate packaging path instead of stretching the macOS DMG flow across incompatible tooling.
7. If a packaging change depends on runtime behavior, call out the dependency explicitly instead of silently folding it into release scripts.
8. Keep touched packaging comments, release notes, and script annotations in professional English.

## Validation

- Verify the requested build artifact exists before packaging.
- Re-run the relevant packaging script after changing release metadata, icon generation, or bundle structure.
- Inspect the final output path and confirm whether signing or notarization was expected or skipped.

## References

- Use `references/packaging-matrix.md` for the current macOS packaging flow, required tools, release environment variables, and Windows x64 release gaps.
