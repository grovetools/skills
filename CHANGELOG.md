## v0.6.0 (2026-02-02)

This release establishes the initial documentation foundation with a first draft of the README (aae32bb) and the addition of the MIT License (293a726). Configuration management has been modernized by migrating to TOML format (d659e30), and new documentation generation capabilities have been added for the skills package (c0cb8d0).

Build and maintenance improvements include fixes to version injection logic (caacc73) and the restoration of the release workflow (a8852db). Additionally, repository organization has been improved by moving documentation rules (f6c3c41) and updating module dependencies (66632df).

### Features
* Readme first draft (aae32bb)
* Add docgen configuration for skills package (c0cb8d0)

### Bug Fixes
* Update VERSION_PKG to grovetools/core path (caacc73)

### Documentation
* Add concept lookup instructions to CLAUDE.md (1020475)

### Chores
* Restore release workflow (a8852db)
* Add MIT License (293a726)
* Migrate grove.yml to grove.toml (d659e30)
* Remove docgen files from repo (ae8f309)
* Move docs.rules to .cx/ directory (f6c3c41)
* Update go.mod for grovetools migration (66632df)

### File Changes
```
 .cx/docs.rules                |  12 +++++
 .github/workflows/release.yml | 123 ++++++++++++++----------------------------
 CLAUDE.md                     |  13 +++++
 LICENSE                       |  21 ++++++++
 Makefile                      |   2 +-
 README.md                     |  54 ++++++++++---------
 docs/01-overview.md           |  32 +++++++++++
 go.mod                        |   9 +++-
 go.sum                        |  49 +++++++++++++++--
 grove.toml                    |   8 +++
 grove.yml                     |   7 ---
 pkg/docs/docs.json            |   8 +++
 12 files changed, 217 insertions(+), 121 deletions(-)
```

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial implementation of grove-skills
- Basic command structure
- E2E test framework
