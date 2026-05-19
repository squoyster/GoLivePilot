# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
