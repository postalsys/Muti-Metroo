# Changelog

## [2.6.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.5.0...muti-metroo-v2.6.0) (2026-01-14)


### Features

* add config-based identity for single-file deployments ([b6dc428](https://github.com/postalsys/Muti-Metroo/commit/b6dc42886b3f0f6866513df778f818b1b3a0b21d))

## [2.5.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.4.3...muti-metroo-v2.5.0) (2026-01-14)


### Features

* remove ICMP allowed_cidrs config option ([2fecf14](https://github.com/postalsys/Muti-Metroo/commit/2fecf1439b234e3b0ab708c2e98d9cb20d8d865e))
* **wizard:** add reverse proxy question and omit empty config values ([c877c90](https://github.com/postalsys/Muti-Metroo/commit/c877c902b1ce098e9d5d6953dadfb23f6271ea97))
* **wizard:** generate minimal config without default values ([ac1edb2](https://github.com/postalsys/Muti-Metroo/commit/ac1edb25274fb560371a6eb80e327ac0f01681ab))

## [2.4.3](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.4.2...muti-metroo-v2.4.3) (2026-01-14)


### Bug Fixes

* trigger release ([f26f6d1](https://github.com/postalsys/Muti-Metroo/commit/f26f6d1efbbd19cdae8722b7d6e0ba6264329984))

## [2.4.2](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.4.1...muti-metroo-v2.4.2) (2026-01-13)


### Bug Fixes

* trigger release for gitignore and header updates ([acd3d1c](https://github.com/postalsys/Muti-Metroo/commit/acd3d1c2730baf40ee40a177c8b6ac0a7efc3e27))

## [2.4.1](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.4.0...muti-metroo-v2.4.1) (2026-01-13)


### Bug Fixes

* trigger release for user manual rename ([bee727f](https://github.com/postalsys/Muti-Metroo/commit/bee727f11969b31c2765983e27080e27dac51376))

## [2.4.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.3.0...muti-metroo-v2.4.0) (2026-01-13)


### Features

* add ICMP echo (ping) support ([bdd164b](https://github.com/postalsys/Muti-Metroo/commit/bdd164bf136df3590176599724c6fbd00a8fa927))
* **icmp:** add IPv6 support, CIDR validation, and security improvements ([db86b52](https://github.com/postalsys/Muti-Metroo/commit/db86b52b90ef8788ffadf033b7e34cb6b9b51fae))


### Bug Fixes

* **chaos:** use sync.Once to prevent double-close panic in MockStream ([a1d7bcf](https://github.com/postalsys/Muti-Metroo/commit/a1d7bcf762b3269a537b3e5f1f307124037534b8))
* **tests:** update tests for TLS and DNS configuration changes ([b7260c5](https://github.com/postalsys/Muti-Metroo/commit/b7260c51f5755819df1e0912c363bb8907ebe2ec))

## [2.3.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.2.0...muti-metroo-v2.3.0) (2026-01-12)


### Features

* **dns:** use system resolver by default instead of public DNS ([baeb18b](https://github.com/postalsys/Muti-Metroo/commit/baeb18bd45c8913e7898ef07a35b415b250e9de6))

## [2.2.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.1.0...muti-metroo-v2.2.0) (2026-01-12)


### Features

* **examples:** add Docker tryout 4-agent example ([83d9637](https://github.com/postalsys/Muti-Metroo/commit/83d9637090f6cd5f447b21eef2cc77141f9d294d))
* **tls:** make certificate verification optional by default ([8552e07](https://github.com/postalsys/Muti-Metroo/commit/8552e072fa6aac059fd9bad5d5035141166e7929))
* **tls:** remove certificate pinning and simplify wizard ([3ac9831](https://github.com/postalsys/Muti-Metroo/commit/3ac983139e4fb0850a05e393c86e5c18ed7320e4))

## [2.1.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.0.1...muti-metroo-v2.1.0) (2026-01-11)


### Features

* add TCP tunnel port forwarding (ngrok-style) ([227b2b0](https://github.com/postalsys/Muti-Metroo/commit/227b2b00fac4af45280872701dbfec084cbe85e8))
* add tunnel visualization to web dashboard ([bd6c668](https://github.com/postalsys/Muti-Metroo/commit/bd6c668f8fd0b30a0027340c24855cbdea019684))
* **dashboard:** show all ingress-exit pairs for port forward routes ([d9a69fe](https://github.com/postalsys/Muti-Metroo/commit/d9a69fee62893b90ea1f751ce0aa9e081193ee6d))


### Bug Fixes

* decode tunnel routes using length-prefixed format ([da64562](https://github.com/postalsys/Muti-Metroo/commit/da645620850443545caef0b9e0305489ce893408))
* update diagram captions in Makefile for renumbered chapters ([e10bcae](https://github.com/postalsys/Muti-Metroo/commit/e10bcae5139d71d9c20b4d41791d94785f496f53))

## [2.0.1](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.0.0...muti-metroo-v2.0.1) (2026-01-10)


### Bug Fixes

* align default reconnect jitter with documented value ([9cbd3e7](https://github.com/postalsys/Muti-Metroo/commit/9cbd3e7b6620ee1bc3fae4b708b3f9a31e7f0534))
* apply gofmt -s and fix IPv6 address format in test ([e90dbc6](https://github.com/postalsys/Muti-Metroo/commit/e90dbc668c9f81a4df1d380a2c5035e0804bb3a1))

## [2.0.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.4.0...muti-metroo-v2.0.0) (2026-01-09)


### ⚠ BREAKING CHANGES

* The `allowed_ports` configuration option has been removed from UDP relay. UDP now allows all ports, matching TCP behavior which never had port restrictions.

### Features

* remove UDP port restrictions to match TCP behavior ([fffb0bf](https://github.com/postalsys/Muti-Metroo/commit/fffb0bf95d75331824eea36e971bd1ccce108c97))

## [1.4.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.3.1...muti-metroo-v1.4.0) (2026-01-09)


### Features

* **embed:** add embedded configuration support for single-file deployments ([d48275f](https://github.com/postalsys/Muti-Metroo/commit/d48275ff1f2abcd24a7ffdd9d7727cd29ffe9915))


### Bug Fixes

* **docs:** add blank lines between chapters in PDF manual ([4315a8a](https://github.com/postalsys/Muti-Metroo/commit/4315a8a40b4b0340bec9569e13bf5236a7da0331))

## [1.3.1](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.3.0...muti-metroo-v1.3.1) (2026-01-09)


### Bug Fixes

* **docs:** add blank lines between chapters in PDF manual ([59ea071](https://github.com/postalsys/Muti-Metroo/commit/59ea071ab7c14e5bf5ad73e37617b8ef0012b86b))

## [1.3.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.2.0...muti-metroo-v1.3.0) (2026-01-09)


### Features

* **docs:** add dynamic version/date to PDF and licensing note ([29b671e](https://github.com/postalsys/Muti-Metroo/commit/29b671e0f8226d2796d0083c45f2d8e2c20c3fad))
* **docs:** add logo to PDF cover and operator manual link to downloads ([ee87666](https://github.com/postalsys/Muti-Metroo/commit/ee876669448a8a8b113be1066a50858f1e346e6f))

## [1.2.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.1.0...muti-metroo-v1.2.0) (2026-01-09)


### Features

* add PDF operator manual with auto-generation on release ([2841533](https://github.com/postalsys/Muti-Metroo/commit/2841533a463a2b35ea1d1c7ce0a7af6da8d02f44))

## [1.1.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.0.34...muti-metroo-v1.1.0) (2026-01-09)


### Features

* add splash page to HTTP API root ([9d71346](https://github.com/postalsys/Muti-Metroo/commit/9d71346979ee262344cdfbc6440df21fefc35be2))

## [1.0.34](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.0.33...muti-metroo-v1.0.34) (2026-01-08)


### Bug Fixes

* **transport:** handle H2 pump write failures to trigger reconnection ([5da6a83](https://github.com/postalsys/Muti-Metroo/commit/5da6a838ba62d46293f8921241fcedb8fbc5df17))

## [1.0.33](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.0.32...muti-metroo-v1.0.33) (2026-01-08)


### Bug Fixes

* add --repo flag to checksums upload command ([19d32a8](https://github.com/postalsys/Muti-Metroo/commit/19d32a8f7ae80b9d9f0f16212a671fb7346ee886))

## [1.0.32](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.0.31...muti-metroo-v1.0.32) (2026-01-08)


### Bug Fixes

* chain build job in release-please workflow ([e796950](https://github.com/postalsys/Muti-Metroo/commit/e796950fe5d14314e94d6da34abd19cb49f336cd))
* test release workflow with chained build jobs ([f338cb2](https://github.com/postalsys/Muti-Metroo/commit/f338cb2016a928016f004d816f168dcc91bf3d99))

## [1.0.31](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v1.0.30...muti-metroo-v1.0.31) (2026-01-08)


### Bug Fixes

* trigger initial release ([ed01738](https://github.com/postalsys/Muti-Metroo/commit/ed017384c062c0fd513e1ead37667e12a84953c2))
