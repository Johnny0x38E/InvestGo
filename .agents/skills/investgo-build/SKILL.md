---
name: investgo-build
description: Build InvestGo frontend and backend artifacts, validate embed correctness, and prepare future cross-platform build work. Use when requests mention building, compilation, release or dev binaries, Wails build flow, frontend/dist embedding, Darwin arm64 or Intel output, or Windows x64 build planning.
---

# InvestGo Build

## Workflow

1. Read `references/build-matrix.md` before changing or running the build pipeline.
2. Always build the frontend before the final Go binary that embeds `frontend/dist`. A stale frontend build can look like a backend regression.
3. Use the standard build and test commands from `AGENTS.md` unless the task specifically needs the macOS release script or a dev binary.
4. Treat `frontend/dist` and `build/bin` as generated outputs, not hand-edited source.
5. When editing build scripts, preserve version injection, icon rendering, and the `--dev` behavior that toggles logging and DevTools.
6. Keep shared preparation separate from platform-specific OS and architecture details. Do not keep OS and arch hardcoded in one script forever.
7. If the change affects packaging, confirm that `scripts/package-darwin-aarch64.sh` still matches the build output path and app metadata contract.
8. Keep any touched script comments, build notes, or generated-file guidance in professional English.

## Validation

- Use the smallest build-related validation set from `AGENTS.md`.
- Run the macOS build script only when the request needs the actual desktop binary or is changing the script itself.

## References

- Use `references/build-matrix.md` for command selection, script behavior, output paths, and platform-specific build gaps.
