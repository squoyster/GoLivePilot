# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.13] - 2026-05-18

### Changed

- Updated `configs/golivepilot.yml` to use `golivepilot-mediamtx.local` as the MediaMTX host for local development.
- Updated HLS preview URLs to use port `8888` on the local host.

## [0.1.12] - 2026-05-18

### Added
- Integrated HLS video player (hls.js) into the web UI to show generated previews.
- Enhanced `StartPreview` to automatically relay the slate feed to MediaMTX's internal ingest point, enabling browser-based verification before going live.

## [0.1.11] - 2026-05-18

### Fixed
- FFmpeg argument construction to properly separate input flags (like `-re`, `-loop`) from output flags (like profiles and format).
- Added `-loglevel` to the FFmpeg command to match configured log level.
- Added debug logging in `RelaySupervisor` to output the final FFmpeg command line for troubleshooting.

## [0.1.10] - 2026-05-18

### Added
- Created `assets/starting-soon.png` as a default slate image.

### Changed
- Updated `configs/golivepilot.yml` to use the relative path for the slate image.

## [0.1.9] - 2026-05-18

### Fixed
- FFmpeg invocation arguments construction to correctly handle profiles and slate looping.
- Added `-re` and looping flags for slate images/videos during preview.
- Improved argument ordering to ensure `-i` is placed correctly relative to input flags.

## [0.1.8] - 2026-05-18

### Changed
- Reverted `configs/golivepilot.example.yml` to use `ffmpeg` as the default binary for Linux compatibility.
- Kept `configs/golivepilot.yml` configured for Apple Silicon macOS (`/opt/homebrew/bin/ffmpeg`).

## [0.1.7] - 2026-05-18

### Changed
- Updated `configs/golivepilot.yml` FFmpeg binary path to `/opt/homebrew/bin/ffmpeg` for macOS.

## [0.1.6] - 2026-05-18

### Fixed
- Resolved an issue where RTMPS URLs placed directly in the `rtmps_url_env` config field were incorrectly reported as empty environment variables.
- Fixed log source path truncation to consistently show only `directory/file.go` even when absolute paths are provided.

## [0.1.5] - 2026-05-18

### Changed
- Truncated source file paths in logs to show only the last two path components (e.g., `app/runtime.go`).
- Improved log consistency by ensuring errors are correctly associated with their contextual loggers.
- Enhanced API handler logging with persistent context for better request tracing.

## [0.1.4] - 2026-05-18

### Changed
- Improved log context by adding `target_id` and `mode` to component-specific loggers.
- Enhanced FFmpeg process logging to include structured fields for better observability.

## [0.1.3] - 2026-05-18

### Changed
- Added source information (file and line numbers) to log entries for better debugging.

## [0.1.2] - 2026-05-18

### Added
- Structured logging using `slog` with support for JSON and text formats.
- Configurable log levels (`debug`, `info`, `warn`, `error`) via `config.yml`.
- API request logging middleware capturing method, path, status, and duration.
- Detailed tracing of FFmpeg process lifecycle and function invocations.

## [0.1.1] - 2026-05-18

### Added
- Implementation of `POST /api/preview` endpoint for starting a slate relay.
- Integrated FFmpeg-based slate relay with `RelaySupervisor` to manage processes.
- Enhanced `/api/status` to include real-time FFmpeg relay status and logs.

### Changed
- Updated `Runtime` and `Server` to support preview workflows and process supervision.

## [0.1.0] - 2026-05-18

### Added
- Initial project structure for GoLivePilot.
- Support for browser-based control UI and FFmpeg-based media handling.
- Configuration examples for GoLivePilot and MediaMTX.

### Changed
- Refactored `main.go` into internal packages (`app`, `config`, `ffmpeg`, `server`) for better modularity.
- Updated README with architectural details and project goals.

### Fixed
- Merged upstream changes from main repository. [c7cbd3e](https://github.com/squoyster/GoLivePilot/commit/c7cbd3ec479c05f2969f90968c7813248cde6303)

## [0.1.17] - 2026-05-18

### Fixed
- Fixed FFmpeg "End of file" errors during preview by adding `-shortest` to ensure output streams are correctly synchronized between the looped slate image and the silent audio stream.
- Refined `BuildArgs` in `internal/ffmpeg/types.go` to ensure output-side arguments (like profiles) are always placed after the main input file.

## [0.1.16] - 2026-05-18

### Fixed
- Fixed FFmpeg AAC encoder failure during preview by forcing the silent audio channel layout to `stereo` (cl=stereo), avoiding unsupported "1 channels (FR)" layouts.

## [0.1.15] - 2026-05-18

### Fixed
- Fixed FFmpeg "Option loop not found" and "Error opening input file" errors by correcting the order of input arguments when silent audio (`anullsrc`) is used.
- Corrected `BuildArgs` logic to ensure global/input flags precede the primary input file correctly.

## [0.1.14] - 2026-05-18

### Changed
- Improved FFmpeg argument construction by explicitly separating input and output options.
- Enhanced FFmpeg log capture to use `bufio.Scanner` for reliable line-by-line logging.
- Refactored `SlateConfig` to include structured `Audio` and `Video` settings.

### Fixed
- Fixed FFmpeg "Broken pipe" errors during preview by adding a silent audio stream when using slates.

[0.1.17]: https://github.com/squoyster/GoLivePilot/compare/v0.1.16...v0.1.17
[0.1.16]: https://github.com/squoyster/GoLivePilot/compare/v0.1.15...v0.1.16
[0.1.15]: https://github.com/squoyster/GoLivePilot/compare/v0.1.14...v0.1.15
[0.1.14]: https://github.com/squoyster/GoLivePilot/compare/v0.1.13...v0.1.14
[0.1.13]: https://github.com/squoyster/GoLivePilot/compare/v0.1.12...v0.1.13
[0.1.12]: https://github.com/squoyster/GoLivePilot/compare/v0.1.11...v0.1.12
[0.1.11]: https://github.com/squoyster/GoLivePilot/compare/v0.1.10...v0.1.11
[0.1.10]: https://github.com/squoyster/GoLivePilot/compare/v0.1.9...v0.1.10
[0.1.9]: https://github.com/squoyster/GoLivePilot/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/squoyster/GoLivePilot/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/squoyster/GoLivePilot/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/squoyster/GoLivePilot/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/squoyster/GoLivePilot/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/squoyster/GoLivePilot/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/squoyster/GoLivePilot/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/squoyster/GoLivePilot/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/squoyster/GoLivePilot/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/squoyster/GoLivePilot/releases/tag/v0.1.0
