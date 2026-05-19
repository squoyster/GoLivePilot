# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.2]: https://github.com/squoyster/GoLivePilot/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/squoyster/GoLivePilot/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/squoyster/GoLivePilot/releases/tag/v0.1.0
