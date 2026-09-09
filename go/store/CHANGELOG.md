# Changelog

## [0.2.0](https://github.com/jrepp/platform/compare/go/store/v0.1.0...go/store/v0.2.0) (2026-09-09)


### ⚠ BREAKING CHANGES

* **store:** MaxBackups > 0 now requires a disjoint same-filesystem BackupRoot; newly generated backups are no longer objects or List entries. All 0.1 callers enabling backups must update configuration. Hosting's version-pinned adoption is prepared; the original Tidal implementation is not silently migrated. Existing public .bakN/temp-looking paths are preserved. Target pre-v1 minor release: go/store/v0.2.0. Store is an existing package; no new domain module is admitted.

### Bug Fixes

* **store:** preserve objects with private atomic backups ([#11](https://github.com/jrepp/platform/issues/11)) ([11116d1](https://github.com/jrepp/platform/commit/11116d13ce58f5c8816cd91b6dbb07013ddbf216))

## 0.1.0 (2026-08-26)


### Features

* **ci:** establish the Go linting discipline for shared packages ([#5](https://github.com/jrepp/platform/issues/5)) ([005ae05](https://github.com/jrepp/platform/commit/005ae0517bf830cfaf1ba1a4326acfe834180950))
* **store:** extract tidal's object store as a fleet package ([#1](https://github.com/jrepp/platform/issues/1)) ([8963000](https://github.com/jrepp/platform/commit/8963000ba80d3e65c051bb07693cfec78a8e4e84))
