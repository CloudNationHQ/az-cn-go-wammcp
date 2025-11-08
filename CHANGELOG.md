# Changelog

## [1.8.4](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.8.3...v1.8.4) (2025-11-08)


### Bug Fixes

* rebuild FTS and vacuum to stop size creep ([#37](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/37)) ([4a386a3](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/4a386a37c44037f3c2bb38f5b965fbd432fe628f))

## [1.8.3](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.8.2...v1.8.3) (2025-11-01)


### Bug Fixes

* remove unused config yml file ([#34](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/34)) ([e625c42](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/e625c42a083b0fa9419e7d5a26b36521e63391fb))

## [1.8.2](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.8.1...v1.8.2) (2025-10-28)


### Bug Fixes

* handle notifications/initialized in MCP protocol handshake ([#32](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/32)) ([c3505c4](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/c3505c4f0beba9ee1d708ec770b5e2c1bf70470d))

## [1.8.1](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.8.0...v1.8.1) (2025-10-21)


### Bug Fixes

* add module_tags schema and fix SQLite ESCAPE in DeleteChildModules ([#30](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/30)) ([97bff33](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/97bff3364a2348470c9677f724a6eb169746826e))

## [1.8.0](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.7.0...v1.8.0) (2025-10-19)


### Features

* some cleanups ([#28](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/28)) ([89318e5](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/89318e51c034b105fed784804244abd4bf81c14f))

## [1.7.0](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.6.0...v1.7.0) (2025-10-18)


### Features

* implement explain relationship tool ([#26](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/26)) ([926ebce](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/926ebce3d0672237c8e6f7cc4b52e6e4473b4f1b))

## [1.6.0](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.5.0...v1.6.0) (2025-10-18)


### Features

* remove category implementation ([#24](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/24)) ([6260559](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/62605590d4f5ff7a3c6fcf868340c872bdc06a3e))

## [1.5.0](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.4.0...v1.5.0) (2025-10-17)


### Features

* structural search filters (kind/type_prefix/has), module info structural summary, and improved related-modules scoring using AST block index ([#22](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/22)) ([7a0e951](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/7a0e951b49e726252ab37b18c4221c373292f4da))

## [1.4.0](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.3.0...v1.4.0) (2025-10-17)


### Features

* add AST-based compare, alias resolution, persisted tags + related-modules, query variants, modernized sync and updated documentation ([#20](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/20)) ([c1caba7](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/c1caba79a70c2c3ed2287118ec21e57a16124287))

## [1.3.0](https://github.com/CloudNationHQ/az-cn-go-wammcp/compare/v1.2.0...v1.3.0) (2025-10-12)


### Features

* rename repository ([#18](https://github.com/CloudNationHQ/az-cn-go-wammcp/issues/18)) ([f4c3733](https://github.com/CloudNationHQ/az-cn-go-wammcp/commit/f4c373358dffaae4e9cc7ef2c42cfd44f60031fa))

## [1.2.0](https://github.com/CloudNationHQ/ac-cn-wam-mcp/compare/v1.1.1...v1.2.0) (2025-10-10)


### Features

* decompose large functions and eliminate code duplication across server, sync, and parser modules ([#13](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/13)) ([ecb3ea8](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/ecb3ea8da18191cfe9f7fb2e097ebb0dc63259a2))
* overhaul sync and indexing for performance and reliability ([#11](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/11)) ([9fd8a4a](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/9fd8a4a08e0d5631d32dbf5b0a3973a887927984))
* parallelize repository sync with worker pool ([#16](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/16)) ([289bf5f](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/289bf5f7b71d1dd16aa65f3e93b408db3af2caa6))
* update documentation ([#17](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/17)) ([b648462](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/b64846227e4a9c8a9d0bcca47259b93d597dc88a))


### Bug Fixes

* fix has_examples flag updates ([#14](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/14)) ([dc3df01](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/dc3df01e084c49363168de98349e7f414daf45ee))
* initialize database on first mcp tool use instead of server startup ([#15](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/15)) ([e192996](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/e1929962efb97cf36d250ecd54b7c6e203afea68))

## [1.1.1](https://github.com/CloudNationHQ/ac-cn-wam-mcp/compare/v1.1.0...v1.1.1) (2025-10-01)


### Bug Fixes

* fetch all pages of github repos ([#8](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/8)) ([6afa3be](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/6afa3be4687d3b8f0c049ec14f6fd50e6211eb9c))
* parse terraform metadata via hcl ast ([#6](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/6)) ([e6a93e7](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/e6a93e72dbe22626cae51a19fba1d82db3736e9a))

## [1.1.0](https://github.com/CloudNationHQ/ac-cn-wam-mcp/compare/v1.0.0...v1.1.0) (2025-10-01)


### Features

* list updated repositories after incremental sync ([#4](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/4)) ([2fc4f87](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/2fc4f878f025010bbbdb26ecdca36569946625dc))


### Bug Fixes

* infer provider from module resources when explicit block missing ([#5](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/5)) ([4023fc7](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/4023fc77aa12b680275f80dd4ac63af26f396108))
* return the correct module id after upsert ([#2](https://github.com/CloudNationHQ/ac-cn-wam-mcp/issues/2)) ([cea6a2d](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/cea6a2d128f12ec932486625154bf5c4d3c6a134))

## 1.0.0 (2025-09-30)


### Features

* add initial files ([9033785](https://github.com/CloudNationHQ/ac-cn-wam-mcp/commit/90337850410c62e278fd833a93ad00e765aed742))
