# tweetdigest
[![GitHub release](http://img.shields.io/github/release/jakewarren/tweetdigest.svg?style=flat-square)](https://github.com/jakewarren/tweetdigest/releases])
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](https://github.com/jakewarren/tweetdigest/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/jakewarren/tweetdigest)](https://goreportcard.com/report/github.com/jakewarren/tweetdigest)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=shields)](http://makeapullrequest.com)
> Compile tweets into a email digest

`tweetdigest` uses the Twitter API to compile tweets from users into a email digest. This digest is intended to be run for specific intervals such as daily.

## Install

This application requires API keys from Twitter (both the consumer API keys and access token). These can be acquired by creating an application from the Twitter developer portal at https://developer.twitter.com/.
Once you have your keys, create a copy of `config.sample.yml` and modify with your values.

### Option 1: Binary

Download the latest release from [https://github.com/jakewarren/tweetdigest/releases/latest](https://github.com/jakewarren/tweetdigest/releases/latest)

### Option 2: From source

```
go get github.com/jakewarren/tweetdigest
```

## Usage

```
tweetdigest --duration "-24h" -c ~/.tweetdigest.yml SwiftOnSecurity
```

## Demo

Screenshot of the sample digest:

![](screenshot.png)

## Changes

All notable changes to this project will be documented in the [changelog].

The format is based on [Keep a Changelog](http://keepachangelog.com/) and this project adheres to [Semantic Versioning](http://semver.org/).

## License

MIT © 2019 Jake Warren

[changelog]: https://github.com/jakewarren/tweetdigest/blob/master/CHANGELOG.md
