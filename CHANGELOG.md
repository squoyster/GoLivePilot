# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.45] - 2026-05-19

### Fixed
- Fixed an issue where `__program_source__` and platform relays could fail with "Invalid argument" due to duplicate output URLs in the FFmpeg command.
- Refactored `BuildArgs` to handle output parameters more robustly and avoid parameter duplication.

## [0.1.44] - 2026-05-19

### Added
- Introduced the `ProgramSwitcher` abstraction to isolate upstream source switching logic.
- Implemented `FFmpegProgramSwitcher` which uses the `Switch` method on the relay supervisor for seamless-ish source transitions.

### Changed
- Refactored `Runtime` to use the new `ProgramSwitcher` for all program source transitions (Slate, Camera, Ended).
- Ensured that platform relays are truly idempotent and persistent; they are started once during Preview and remain running throughout the broadcast.
- Improved the separation of concerns by removing direct FFmpeg command building from the `Runtime` core.

## [0.1.43] - 2026-05-19

### Added
- Introduced a two-stage "Stop" process: "Stop Stream" now transitions to an "Ended" slate while keeping platform connections live, and "Reset Everything" performs a hard stop.
- Added a `HardStop` method to `Runtime` and exposed it via `/api/stop?hard=true`.

### Changed
- Improved stream stability by ensuring platform relays are NOT stopped during the transition back to Preview or between Slate/Camera.
- Refactored `StartPreview` to maintain existing platform relays, allowing for seamless source switching without dropping the Facebook/YouTube session.
- Updated the frontend UI with clearer "Stop Stream" and "Reset Everything" buttons and improved status feedback.

## [0.1.42] - 2026-05-19

### Added
- Implemented comprehensive unit tests for all core packages: `config`, `ffmpeg`, `mediamtx`, `app`, and `server`.
- Achieved high test coverage for critical components like `ProcessSupervisor`, `Config` validation, and MediaMTX client.
- Added mock-based testing for `Runtime` and `Server` to verify business logic and API endpoints.

## [0.1.41] - 2026-05-19

### Added
- Created a `mediamtx.Client` to interact with the MediaMTX API and wait for stream readiness.

### Changed
- Refactored `ProcessSupervisor` to allow restarting failed or exited relays without "already running" errors.
- Added `lastStatus` to `ProcessSupervisor` to preserve failed relay logs and status history.
- Improved `Runtime` sequencing: platform relays are now gated by `live/program` readiness.
- Added explicit readiness check for `live/camera` before transitioning to "Go Live".
- Reduced stop timeout for faster switching and better responsiveness.

## [0.1.40] - 2026-05-19

### Added
- Implemented a durable 'program' feed architecture using MediaMTX to ensure stable RTMPS sessions during source switching.
- Introduced `ProgramConfig` to define stable publish, source, and HLS URLs for the program feed.
- Added support for multiple slate images: `standby_image`, `starting_image`, and `ended_image`.

### Changed
- Refactored `Runtime` to decouple platform relays from program source switching.
- Platform relays now use a stable `live/program` source and remain running during the transition from Slate to Camera.
- Updated MediaMTX configuration with `overridePublisher: true` for `live/program` and `live/preview` paths.
- Renamed `SourceMode` constants for better clarity (`SourceStandby`, `SourceSlate`, `SourceCamera`, `SourceEnded`, `SourceStopped`).
- Updated frontend UI to handle new state constants and consistently reflect the "program" feed status.

## [0.1.39] - 2026-05-19

### Changed
- Aligned FFmpeg streaming parameters with Facebook's recommended settings (2.5Mbps bitrate, 30fps, specific GOP size).
- Refactored `StartPreview` and `StartGoLive` in `Runtime` to use consistent and optimized encoding parameters.
- Updated the `x264-720p` profile to match industry-standard streaming requirements for social platforms.
- Improved `BuildArgs` in the FFmpeg package to handle `-f flv` duplication gracefully.

## [0.1.38] - 2026-05-19

### Changed
- Updated the "Preview", "Go Live", and "Stop" buttons to use Blue, Green, and Red colors respectively.
- Refactored the control buttons layout to be mobile-friendly, using a vertical stack and increased spacing for the "Stop" button on small screens to prevent accidental clicks.

## [0.1.37] - 2026-05-19

### Added
- Customized command-line help (`--help`) to show a clear usage summary and a reference to `--env-help`.

## [0.1.36] - 2026-05-19

### Added
- Added a new command-line flag `--env-help` to display relevant environment variables, their descriptions, and format examples.
- Introduced a central `RelevantEnvVars` registry in the configuration package for documenting supported environment variables.

## [0.1.35] - 2026-05-19

### Added
- Integrated `standing-by.png` as the dedicated placeholder for the "Standby" (initialized) state.

### Changed
- Updated the video viewer to distinguish between "Standby" (using `standing-by.png`), "Previewing" (using `starting-soon.png`), and "Ended" (using `stream-ended.png`) states.

## [0.1.34] - 2026-05-19

### Changed
- Improved RTMPS target configuration by separating the base URL from the stream key.
- Refactored `TargetConfig` to include a dedicated `rtmps_key_env` field.
- Updated Facebook configuration to use the new separated URL and key format for better compatibility with FFmpeg.

## [0.1.33] - 2026-05-19

### Fixed
- Fixed an issue where the video stream was not correctly picked up when transitioning to "Go Live".
- Switched "Go Live" relay to use stable transcoding instead of copy-only, ensuring better compatibility with HLS/RTMPS targets.
- Improved frontend player reliability by adding a delay before reloading and preventing error recovery from overriding visual status.

## [0.1.32] - 2026-05-19

### Fixed
- Fixed a frontend regression where UI elements (status stepper, video feed, labels) failed to update correctly.
- Corrected JavaScript scoping issues in the `StatusManager` implementation and added defensive checks for DOM elements.
- Ensured the HLS player is properly initialized and managed during state transitions.

## [0.1.31] - 2026-05-19

### Added
- Introduced a central `StatusManager` in the frontend to handle event-driven UI updates.
- Added a visual status stepper to track and highlight the current stream state (Standby -> Preview -> Go Live -> Stream Ended).
- Enhanced the video viewer header to dynamically display the current stream mode.

### Fixed
- Fixed inconsistent UI updates by abstracting status tracking and DOM manipulations.
- Corrected the `stream-ended.png` placeholder logic to ensure it displays correctly when the stream ends.

## [0.1.30] - 2026-05-18

### Added
- Integrated `stream-ended.png` as a dedicated placeholder for the "Ended" state.

### Changed
- Refined the video viewer to distinguish between "Initialized" (using `starting-soon.png`) and "Ended" (using `stream-ended.png`) states visually.

## [0.1.29] - 2026-05-18

### Fixed
- Fixed a bug where the status overlay would get stuck on "Connecting..." and fail to update when transitioning to LIVE or Ended.
- Ensured backend `source_mode` always takes precedence over transient frontend player states.
- Improved RTMP/S URL resolution logic in `Runtime` to better handle direct URLs in configuration.

## [0.1.28] - 2026-05-18

### Added
- Introduced `initialized` state in `SourceMode` to track the initial state of the application before any stream starts.

### Changed
- Renamed the "Preview" section in the UI to "Live Stream Viewer" for better clarity.
- Updated the status overlay to show "Initialized" upon startup.
- Improved the status polling logic to correctly transition between "Initialized", "Preview", "LIVE", and "Ended" without getting stuck on "Connecting".
- Ensured the placeholder image is displayed during the "Initialized" state.

## [0.1.27] - 2026-05-18

### Added
- Added `/assets/` endpoint to serve static files from the `assets` directory.
- Introduced a placeholder image (`starting-soon.png`) that displays in the video viewer when the stream is stopped/ended.

### Changed
- Enhanced the video viewer overlay to dynamically reflect the stream status (Preview, LIVE, Ended) based on backend state.
- Improved overlay styling with better contrast, rounded corners, and a specific "LIVE" visual state (red text).
- Refactored frontend polling logic to properly handle stream termination by stopping the HLS player and switching to the placeholder image.

## [0.1.26] - 2026-05-18

### Added
- Implemented "Go Live" source switching between slate and camera modes.
- Introduced `SourceMode` (slate, camera, none) to track runtime state.
- Added `ui.camera_source_url` configuration to support local camera ingestion.

### Changed
- Refactored `Runtime` to manage source transitions and ensure stable stream handovers.
- Updated `/api/status` to include the current `source_mode`.
- Wired up `/api/go-live` to switch relays from slate to camera with stream copy.

## [0.1.25] - 2026-05-18

### Fixed
- Fixed a UI issue where the preview player would remain stuck on "Connecting" even after the video feed started.
- Added a `playing` event listener to ensure the status correctly updates to "Live Preview" as soon as media playback begins.

## [0.1.24] - 2026-05-18

### Fixed
- Fixed the HLS preview player requiring a manual page refresh to load the stream.
- Implemented automatic player reloading when clicking the "Preview" or "Go Live" buttons.
- Added a robust retry mechanism for both native HLS and Hls.js to handle stream startup delays.
- Enhanced Hls.js configuration with tuned retry intervals and low-latency settings for better responsiveness.

## [0.1.23] - 2026-05-18

### Added
- Added a configurable `preview_hls_url` field to `UIConfig` for the web application.

### Changed
- Integrated a visible HLS video player into the main UI for live verification of slate relays.
- Enhanced the HTML template to support both native browser HLS playback and Hls.js fallback with automatic error recovery and logging.
- Updated example and local configurations to include the default MediaMTX HLS preview URL.

## [0.1.22] - 2026-05-18

### Fixed
- Fixed the HLS preview player to correctly point to the `live/preview` path when MediaMTX is used, ensuring the slate broadcast is visible in the UI.

## [0.1.21] - 2026-05-18

### Fixed
- Fixed a bug in `StartPreview` where an empty `LOCAL_PREVIEW_RTMP_URL` environment variable would block all preview relays from starting.
- Improved error handling in `StartPreview` to continue starting other target relays even if one target's configuration is invalid.

## [0.1.20] - 2026-05-18

### Changed
- Refactored `StartPreview` to iterate through all enabled targets and initiate slate relays to all of them.
- Updated `configs/golivepilot.yml` and `configs/golivepilot.example.yml` to include a dedicated local browser preview target.
- Standardized FFmpeg slate relay parameters for maximum compatibility with both local and external targets.

### Fixed
- Fixed FFmpeg argument ordering and duplication issues in `BuildArgs`.
- Corrected the local browser preview output path to `live/preview` to avoid conflicts with camera ingest.

## [0.1.19] - 2026-05-18

### Fixed
- Stabilized FFmpeg preview relay by adopting proven command-line parameters for slate broadcasting.
- Fixed stream synchronization issues by implementing explicit `-map` directives for video and silent audio inputs.
- Optimized image slate encoding with `-tune stillimage` and standardized 30fps output.
- Simplified `BuildArgs` logic to improve reliability of generated FFmpeg commands.

## [0.1.18] - 2026-05-18

### Fixed
- Improved FFmpeg slate relay stability by explicitly setting the `-framerate` for image slates and refining `-shortest` usage.
- Refactored `BuildArgs` to more reliably handle multiple input files (e.g., silent audio and slate image) while maintaining correct argument ordering.

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

[0.1.27]: https://github.com/squoyster/GoLivePilot/compare/v0.1.26...v0.1.27
[0.1.26]: https://github.com/squoyster/GoLivePilot/compare/v0.1.25...v0.1.26
[0.1.25]: https://github.com/squoyster/GoLivePilot/compare/v0.1.24...v0.1.25
[0.1.24]: https://github.com/squoyster/GoLivePilot/compare/v0.1.23...v0.1.24
[0.1.23]: https://github.com/squoyster/GoLivePilot/compare/v0.1.22...v0.1.23
[0.1.22]: https://github.com/squoyster/GoLivePilot/compare/v0.1.21...v0.1.22
[0.1.21]: https://github.com/squoyster/GoLivePilot/compare/v0.1.20...v0.1.21
[0.1.20]: https://github.com/squoyster/GoLivePilot/compare/v0.1.19...v0.1.20
[0.1.19]: https://github.com/squoyster/GoLivePilot/compare/v0.1.18...v0.1.19
[0.1.18]: https://github.com/squoyster/GoLivePilot/compare/v0.1.17...v0.1.18
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
