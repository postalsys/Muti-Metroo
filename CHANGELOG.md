# Changelog

## [2.16.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.15.0...muti-metroo-v2.16.0) (2026-01-27)


### Features

* **health:** add mesh connectivity test endpoint and CLI ([2d0faa6](https://github.com/postalsys/Muti-Metroo/commit/2d0faa673ecc52f2c259499fbf09434e25a8458c))


### Bug Fixes

* **agent:** resolve mutex race conditions in stream handlers ([a74b266](https://github.com/postalsys/Muti-Metroo/commit/a74b266fbdd69fa3a5d73c67661f4e681546ee2c))
* **security:** restrict topology exposure when management key lacks private key ([8f174f9](https://github.com/postalsys/Muti-Metroo/commit/8f174f9ad054b62c2de7951b1f916984ce8681d6))
* **test:** use dynamic port allocation in integration tests ([1b95930](https://github.com/postalsys/Muti-Metroo/commit/1b95930f0e465c3260410b6c021fc72a80f878f0))

## [2.15.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.14.0...muti-metroo-v2.15.0) (2026-01-23)


### Features

* **sleep:** add Ed25519 signature verification for sleep/wake commands ([6bee8ed](https://github.com/postalsys/Muti-Metroo/commit/6bee8ed8545d4888952bf9734028c53a67b25e7e))


### Bug Fixes

* **deps:** update quic-go and x/crypto to fix security vulnerabilities ([f6db21d](https://github.com/postalsys/Muti-Metroo/commit/f6db21d411ea133c05dec440fc4e8205bf3dfb87))

## [2.14.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.13.0...muti-metroo-v2.14.0) (2026-01-23)


### Features

* **probe:** add test listener subcommand and plaintext WebSocket support ([fa0d7b2](https://github.com/postalsys/Muti-Metroo/commit/fa0d7b231f064bf841e0a21acc8f22a01fb5061c))
* **service:** deploy binary to system location during installation ([3e384af](https://github.com/postalsys/Muti-Metroo/commit/3e384afe36decf5645bacb062ad6ef0d932e68f2))


### Bug Fixes

* **wizard:** add clarifying notes about config file and data directory ([7b618a1](https://github.com/postalsys/Muti-Metroo/commit/7b618a1c21fbaff9c772ae40f79fbc97d908ccb6))
* **wizard:** offer user service installation for non-root Linux users ([decf4d2](https://github.com/postalsys/Muti-Metroo/commit/decf4d28afb5f9c0f47bcc57996f5294be8b9175))

## [2.13.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.12.0...muti-metroo-v2.13.0) (2026-01-19)


### Features

* **sleep:** add mesh hibernation mode ([57d271e](https://github.com/postalsys/Muti-Metroo/commit/57d271e74ce2d181e2eca808253a737a2cfb7bbb))


### Bug Fixes

* **sleep:** resolve wake race condition during poll cycles ([5207628](https://github.com/postalsys/Muti-Metroo/commit/5207628be25fafe16ee11ff9b49213af668089c6))

## [2.12.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.11.3...muti-metroo-v2.12.0) (2026-01-19)


### Features

* add traffic analysis lab with Suricata IDS ([4914767](https://github.com/postalsys/Muti-Metroo/commit/4914767241956afc1fa65c1507b5bee2755c369d))
* **transport:** add TLS fingerprint customization for JA3/JA4 evasion ([32f4172](https://github.com/postalsys/Muti-Metroo/commit/32f41720805b2ca576f37a4e48c0851126d6c424))

## [2.11.3](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.11.2...muti-metroo-v2.11.3) (2026-01-17)


### Bug Fixes

* **service:** include data directory in systemd ReadWritePaths ([f422284](https://github.com/postalsys/Muti-Metroo/commit/f422284c5d725961c083c0915538befcfe2aa205))

## [2.11.2](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.11.1...muti-metroo-v2.11.2) (2026-01-17)


### Bug Fixes

* **service:** fix Windows user service rundll32 argument passing ([8039cae](https://github.com/postalsys/Muti-Metroo/commit/8039cae9db49f59ecc8e43d0c153f5bbe6fd265a))

## [2.11.1](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.11.0...muti-metroo-v2.11.1) (2026-01-17)


### Bug Fixes

* **service:** add service info display and improve documentation ([3f578dd](https://github.com/postalsys/Muti-Metroo/commit/3f578dd7ec1e68b9a033fbaf03cb67601b929b2f))
* **service:** improve Windows process detection and restore required flag ([47d78c5](https://github.com/postalsys/Muti-Metroo/commit/47d78c5cc93b0365974bcfaa4c64470a98346689))

## [2.11.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.10.0...muti-metroo-v2.11.0) (2026-01-17)


### Features

* **service:** add Windows user service via Registry Run key ([b9605d2](https://github.com/postalsys/Muti-Metroo/commit/b9605d2fdfc3b7b22a13be199d759c86fc588e69))

## [2.10.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.9.1...muti-metroo-v2.10.0) (2026-01-16)


### Features

* **dll:** add UPX compression, disable config embedding for DLLs ([fb6e1f5](https://github.com/postalsys/Muti-Metroo/commit/fb6e1f5742b9c2e5b72fae1141de86b36a3548d3))


### Bug Fixes

* **wizard:** skip file creation when embedding config to target binary ([b2b8944](https://github.com/postalsys/Muti-Metroo/commit/b2b8944a0b18b818ffe88ecec959b0a7302c7cc2))

## [2.9.1](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.9.0...muti-metroo-v2.9.1) (2026-01-16)


### Bug Fixes

* trigger release ([c9c260e](https://github.com/postalsys/Muti-Metroo/commit/c9c260e344eea173bee194ee487d5f66fac2a6e1))

## [2.9.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.8.0...muti-metroo-v2.9.0) (2026-01-16)


### Features

* add Windows DLL build for rundll32.exe execution ([996a6bf](https://github.com/postalsys/Muti-Metroo/commit/996a6bf088fdc2821a4eaa437629a115048ae51a))


### Bug Fixes

* **user-manual:** update port forwarding diagram reference ([7f229ab](https://github.com/postalsys/Muti-Metroo/commit/7f229ab1520d9ee040cdfc0531dcb87bc41d0e85))

## [2.8.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.7.0...muti-metroo-v2.8.0) (2026-01-16)


### Features

* **socks5:** add WebSocket transport with HTTP Basic Auth ([648c46b](https://github.com/postalsys/Muti-Metroo/commit/648c46bfd409e7d45ad5ffab933ba5262825eaf6))


### Bug Fixes

* use Infima utility classes for intro CTA button ([aba0a6e](https://github.com/postalsys/Muti-Metroo/commit/aba0a6e8e2128467f614005e27e6881cf7b2c94e))

## [2.7.0](https://github.com/postalsys/Muti-Metroo/compare/muti-metroo-v2.6.0...muti-metroo-v2.7.0) (2026-01-15)


### Features

* add configurable default CLI action for embedded configs ([c22e812](https://github.com/postalsys/Muti-Metroo/commit/c22e812b2b349963a57378b4ee8af898a232070a))

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
