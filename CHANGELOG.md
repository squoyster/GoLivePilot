# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
