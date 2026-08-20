# Route plugin fixtures

Source files compiled into real Go plugins by `TestPluginRoutesEndToEnd`
(`routing/plugin_integration_test.go`), which is the only test exercising the
real plugin seam of the framework: `go build -buildmode=plugin`, then
`plugin.Open`, symbol extraction and route registration through the real
`loadAPIHandlers`.

They live under `testdata/` on purpose: the go tool ignores that directory, so
`go build ./...` and `go vet ./...` never compile these `package main` files —
the test compiles them one by one, from the module itself, so that each plugin
shares the toolchain, the dependency versions and the package import paths of
the test binary.

Each fixture gets its own `.so`, and each `.so` is opened from a single path:
the runtime rejects a second plugin carrying a plugin path it already loaded
(`plugin already loaded`), which is what copying one compiled `.so` to two
locations would produce.

Layout, as copied into the temporary API folders of the test:

| fixture                | API path                          | route                | purpose                                                    |
| ---------------------- | --------------------------------- | -------------------- | ---------------------------------------------------------- |
| `widgets/list.go`      | `api/widgets/list.go`             | `GET /api/widgets`   | `func(*gin.Context)` signature                             |
| `widgets/create.go`    | `api/widgets/create.go`           | `POST /api/widgets`  | `func(*core.Context)`, second method of the same folder    |
| `widgets/id_/routes.go`| `api/widgets/id_/routes.go`       | `/api/widgets/:id`   | typed params, by value (`GET`) and by pointer (`PATCH`)     |
| `public/index.go`      | `api/public/index.go`             | `GET /api/public`    | `Permissions` declaring `authorization.Public`             |
| `orphan/index.go`      | `api_broken/widgets/index.go`     | —                    | `Permissions` entry naming no handler: boot must fail       |
| `widgets/list.go`      | `api_missing/widgets/index.go`    | —                    | copied without being compiled: boot must fail              |
