# Changelog

All notable changes to this project will be documented in this file. See [commit-and-tag-version](https://github.com/absolute-version/commit-and-tag-version) for commit guidelines.

## 0.2.0 (2026-10-19)


### Features

* **xttp:** security headers always on: helmet set plus `Cache-Control: no-store` on every response, also on 413/431 written before routing and on probes; no `Server` banner by default (`HideServerHeader`); `HelmetPreset: HelmetStrict` (deny-all CSP, `X-Frame-Options: DENY`, Permissions-Policy all off, preloaded HSTS on TLS); `Config.Helmet` overrides one field, `"-"` removes a header; `Config.Headers` for extra static headers
* **xttp:** `StatusOf(err)` for middleware that needs the response status of a returned error
* **xttp:** `ErrorDocument` / `ErrorObject` exported; error responses documented as `application/vnd.api+json`; `Config.DefaultErrors` adds them to every route
* **xttp:** `required:"true|false"` tag on any struct field
* **xttp:** `Config.Docs` (Enabled, Path, Page, Guard) and `Config.Profiling` (pprof behind a required Guard); the docs page carries its own CSP so it renders under `HelmetStrict`
* **xttp:** `Config.Logger`, `Config.AccessLog`, `(*API).ErrorHandler`: no package-level logger
* **xttp:** `Config.RateLimitHeaders` (default off: no `X-RateLimit-*`) and `Config.StrictContentType` (415 for non-JSON bodies)
* **xttp:** an empty `Route.Path` on a group registers the group prefix itself
* **xttp:** `example/strict`, the hardened reference server for the VAPT checklist
* **envy:** `Defaults(ptr)` applies only `default` tags; warnings go through `slog` instead of stdout
* **climd:** `AppConfig.Default`, the command run without arguments


### Bug Fixes

* **xttp:** error responses were documented as `application/json` while sent as `application/vnd.api+json`
* **xttp:** request body fields were always required
* **xttp:** `Run` listened on `tcp4` only; the default is now dual-stack `tcp` (set `Listen.ListenerNetwork` to keep IPv4 only)
* **xttp:** pre-routing errors (413, 431) missed the security headers


### Behaviour changes

* every response now carries `Cache-Control: no-store` and the helmet headers, and no `Server` header; `Headers: {"Cache-Control": "-"}` or `HideServerHeader: &f` restore v0.1.0
* `X-RateLimit-*` headers are no longer sent unless `RateLimitHeaders: &t`
* a `Responses` entry ≥ 400 with a `nil` body is now documented as `ErrorDocument`
* the listener accepts IPv6

## 0.1.0 (2026-10-18)


### Features

* add foreverwork ([bd6e3f9](https://github.com/moeghifar/libgo/commit/bd6e3f9d76e3ab0d25543750cf7f7429b7a4f5e2))
* add new climd readme ([f5afbab](https://github.com/moeghifar/libgo/commit/f5afbab3b3cba41943d822761d4e2bad5b1b01dd))
* add xttp, Fiber v3 routes with OpenAPI 3.1, JSON:API errors and server defaults ([94deab3](https://github.com/moeghifar/libgo/commit/94deab311079785e70c55d1671575d87b1408ba0))
* climd as Cobra replacement ([5531017](https://github.com/moeghifar/libgo/commit/55310176b49dcd7375637066248f54903c93a493))
* initial setup for env ([34c2e0d](https://github.com/moeghifar/libgo/commit/34c2e0dfa732becd59d81020a3cd307827920f09))
* next gen with dotenv ([d888d8a](https://github.com/moeghifar/libgo/commit/d888d8a4767bf77e66cbabb71439c9ee0b80cf42))
