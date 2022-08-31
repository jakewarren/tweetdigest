# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] - 2022-08-31
### Fixed
- fixed a nil reference error when attempting to unshorten an URL that cannot be reached

## [0.3.0] - 2022-04-11
### Added
- added support for creating a digest with multiple users
- added option to ignore reply tweets (`--include-replies`)
### Changed
- set the DNT (Do Not Track) signal
### Fixed
- fix expansion of retweets. Twitter implemented changes so you have to use the API to get tweet content.
- fixed a bug that was preventing replies in a user's thread from being displayed

## [0.2.0] - 2020-02-24
### Added
- added a feature to unshorten URLs
- expand quoted tweets
- add a parameter to control verbose output
- add a parameter to exclude retweets

## [0.1.2] - 2019-11-14
### Changed
- adjusted the sort order so oldest tweets appear first in the email

## [0.1.1] - 2019-11-07
### Changed
- make the email server information configurable
- all the `to` addresses to be configurable via parameters
- make the amount of tweets pulled from Twitter configurable

## [0.1.0] - 2019-11-07
- Initial Release

[unreleased]: https://github.com/jakewarren/tweetdigest/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/jakewarren/tweetdigest/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/jakewarren/tweetdigest/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/jakewarren/tweetdigest/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/jakewarren/tweetdigest/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/jakewarren/tweetdigest/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/jakewarren/tweetdigest/releases/tag/v0.1.0
