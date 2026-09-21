# AGENTS.md

## Overview

A Go CLI (`immich-album-generator`) that queries an Immich photo server and syncs
server-side assets into named albums ("Recent", "Memories", "People"). When more
assets match than the configured limit allows, it picks a subset with
exponential-decay weighted random sampling so older assets still appear
occasionally rather than being cut off by a hard date sort.

The tool is designed to run repeatedly (e.g. nightly via cron): each run
recomputes the desired album contents from scratch and diffs them against the
current album state.

## Commands

```bash
go build -o bin/immich-album-generator .   # build output goes to bin/ (gitignored)
go run . --help                            # run without building
go test ./...                              # all tests
go test ./pkg/selector/ -run TestCalculateDecayWeight -v   # single test
go vet ./...                               # vet (currently clean)
go build ./...                             # typecheck only
```

There is no Makefile, no CI config, and no linter config. Built binaries go to
`bin/`, which is gitignored; rebuild if you change source. The module requires Go
1.26.1 (`go.mod`). `README.md` documents usage for humans.

## Running

Credentials come from `--url` / `--api-key` flags, defaulting to the `IMMICH_URL` /
`IMMICH_API_KEY` env vars. `cmd/root.go` calls `godotenv.Load()` in `init()`, so a
`.env` file in the working directory is picked up automatically. Both values are
required; the run aborts with an error if either is empty.

```bash
IMMICH_URL=https://immich.example.com IMMICH_API_KEY=xxx go run . --dry-run
go run . --people "Alice,Bob" --people-album-name "Family" --recent-limit 0
```

All flags live on the single root cobra command, prefixed per album type:
`--recent-days/--recent-limit/--recent-half-life/--recent-album-name`, the same
`--memories-*`, plus `--people`/`--people-limit`/`--people-half-life`/
`--people-album-name` (People has no days flag), and the globals `--dry-run` and
`--verbose`. There are no subcommands. `--verbose` sets `Client.Verbose`, making
`doRequest` log the method, URL, and pretty-printed JSON request/response bodies to
stderr (prefixed `Immich:`); it never logs the API key. `Client.LogWriter` overrides
the destination and exists so tests can capture the output. Defaults: recent 90
days/5000 items/half-life 30; memories 14 days/5000 items/half-life 7; people 5000
items/half-life 3650.

Enable/disable logic in `runFunc`: the Recent block runs only when `recentLimit > 0`,
Memories only when `memoriesLimit > 0`, People only when `--people` is non-empty.
Setting a limit to `0` is the way to skip an album type.

## Architecture and data flow

```
main.go -> cmd/root.go (cobra flags, wiring)
             |
             v
        pkg/sync/strategy.go   SyncRecent / SyncMemories / SyncPeople
             |            \
             v             v
   pkg/selector/*.go    pkg/immich/*.go      pkg/sync/album.go
   (pure weighting)     (HTTP client,          (EnsureAlbum,
                         API endpoints)         SyncAlbumAssets diff)
```

Each `Sync*` function follows the same four steps:

1. **Fetch** candidate assets from Immich (`pkg/immich/api.go`).
2. **Weight** via `pkg/selector` (returns assets and a parallel weights slice, 1:1
   indexed).
3. **Select** `limit` assets via `selector.SelectWeightedRandom`.
4. **Sync** via `sync.EnsureAlbum` then `sync.SyncAlbumAssets`.

Date-window filtering belongs in the query, not in `pkg/selector`: `ProcessRecent`
and `ProcessMemories` only compute weights, because the API searches already return
exactly the wanted date range.

`pkg/selector` is deliberately pure and dependency-free (no HTTP, no clock
injection except `time.Now()`): its tests need no HTTP server. Keep it that
way — add logic here rather than in `pkg/sync` when it can be expressed as a pure
function over assets/dates, because it is testable without a live Immich server.

### Selection algorithm

`selector.go` implements A-Res weighted random sampling without replacement:
key = `u^(1/w)` with `u` uniform in (0,1], then sorts descending and takes the top
`limit`. `CalculateDecayWeight` returns `0.5^(distance/halfLife)`. Keys are
computed with `math/rand` globals — there is no seeding, so results are not
reproducible across runs. This is intentional (the "screensaver" effect needs
different results each run); do not add seeding without asking.

### Two different "distance" semantics

- `CalculateDaysDistance`: absolute day difference after normalizing both times to
  UTC midnight. Used for Recent and People (raw asset age).
- `CalculateMemoriesDistance`: ignores the year, comparing month/day against today's
  month/day in the current, previous, and next year and taking the minimum. This
  makes Jan 1 vs Dec 31 a distance of 1 day rather than 364. Covered by tests in
  `distance_test.go` — preserve the New Year wrap behavior.

- `ProcessMemories` drops anything whose `FileCreatedAt.Year() >= today.Year()`, so
  assets from the current year never count as memories. This is the one filter the
  search query cannot express: when today is near the New Year, the previous year's
  query window (`targetDate ± maxDays`) spills into the current year.

## Immich API client

`doRequest` is the single choke point: it marshals a JSON body, sets the
`x-api-key` header, and returns an error for any status >= 400 with the response
body embedded. 30s HTTP timeout.

**Dual-endpoint fallbacks are intentional, not dead code.** Immich renamed endpoints
across versions, so `pkg/immich/album.go` tries `/api/albums*` first and falls back
to the singular `/api/album*` on error (GetAlbumByName, GetAlbumInfo, CreateAlbum,
AddAssetsToAlbum, RemoveAssetsFromAlbum). When adding an endpoint that exists in
both forms, follow this pattern.

**`searchMetadata` paginates until exhausted, and parsing is dual-shape.** Every
`/api/search/metadata` call goes through the unexported `searchMetadata` helper in
`pkg/immich/api.go`, which owns the request and the dual-shape parsing (nested
`{"assets": {"items": [...]}}` envelope, then flat `[]Asset` array fallback).
`GetAssetsByPeople`, `SearchAssetsByDateRange`, and `GetAlbumInfo` are thin wrappers
that only build the `SearchMetadataRequest`. Add new search criteria as fields on
`SearchMetadataRequest` and call the helper rather than hand-rolling a request.
`GetAlbumInfo` re-queries with `albumIds` (an array) when the album detail response
contains no assets: Immich's album detail schema only exposes `assetCount`, not the
assets, so the search fallback is the real source of album members. `GetAssetsByPeople` issues one search per person and merges the results,
deduplicating by asset ID: Immich applies `personIds` conjunctively (an asset must
contain every listed person), so a single request would return the intersection
rather than the union the caller wants.

**Pagination is handled inside the helper, so callers always get every matching
asset, never just the first page.** `/api/search/metadata` is paginated and Immich
changed its paging protocol in v3.2.0, so the loop supports both:

| | request field | response field under `assets` |
|---|---|---|
| Immich >= v3.2.0 | `cursor` (opaque) | `nextCursor` |
| older Immich | `page` (1-based) | `nextPage` (next page number as a string) |

The loop sends `size: 1000` (Immich's maximum), starts on `page: 1`, and prefers
`nextCursor` whenever the server returns one. It stops when a page comes back empty,
when no token is returned, or when the server repeats the token it was just given
(guards against a server that ignores the cursor and would otherwise loop forever).
Covered by `pkg/immich/api_test.go`, which stubs the endpoint with `httptest` and
asserts on the request bodies sent per page.

Endpoint/type reference:
- `POST /api/search/metadata` — search by `personIds`, date range, or `albumIds`
  (array). The scalar `albumId` is not a valid field and is silently ignored.
- `GET /api/search/person?name=`, `GET /api/people/{id}` — resolve people. Name
  search is a GET with the name as a query parameter, not a POST body.
- `GET/POST /api/albums`, `GET /api/albums/{id}` — album CRUD. The list filter is
  `?name=` (exact match); `albumName` is not a valid query parameter.
- `PUT /api/albums/{id}/assets`, `DELETE /api/albums/{id}/assets` — add/remove
  assets. The body is `BulkIdsDto`, i.e. `{"ids": [...]}`, not `{"assetIds": [...]}`.
  Removal intentionally sends a JSON body on DELETE; Immich accepts it and
  `doRequest` supports it. Do not "fix" this to query params.

## Sync semantics

`SyncAlbumAssets` (`pkg/sync/album.go`) fetches the album's current assets, builds
ID sets, computes `toAdd` and `toRemove`, and applies them. **This means assets not
chosen in the current run are removed from the album**, so the album always
converges to exactly the selected set. Any change to selection changes album
membership in both directions — check the remove path when touching selection.

### Dry-run

`dryRun` threads down through every `Sync*` call and is handled in two places:
`EnsureAlbum` returns a synthetic album with the magic ID `"dry-run-album-id"`, and
`SyncAlbumAssets` compares `albumId != "dry-run-album-id"` to skip the info fetch,
then logs intended adds/removes and returns before any mutation. If you add a new
write path, gate it on `dryRun` explicitly — there is no central interceptor.

## Gotchas

- `runFunc` prints per-album errors with `fmt.Printf` and **continues to the next
  album, returning nil overall**. A failed album therefore exits with status 0.
  Don't assume a zero exit means everything synced.
- `SelectWeightedRandom` indexes `weights[i]` alongside `assets` with no length
  check; mismatched slices panic. `ProcessMemories` must append to both slices
  together; `ProcessRecent`/`ProcessPeople` return `assets` unmodified, so their
  weights slice must have exactly `len(assets)` entries.
- `parseSearchResult` in `pkg/immich/api.go` treats the `assets` key being present
  as authoritative: an envelope with zero items parses to an empty slice, not an
  error. Only genuinely unrecognizable payloads error. Be aware that pagination
  therefore propagates an empty result rather than failing, and the destructive
  remove path in `SyncAlbumAssets` will empty an album if a search legitimately
  matches nothing.
- `SyncMemories` loops 50 years back one year at a time, issuing a separate search
  per year (50 HTTP requests per run). Deliberate, since a single wide range would
  pull in unrelated months.
- Weights are computed from `FileCreatedAt`, not album-add time or upload time.
- Date-range searches send `takenAfter`/`takenBefore` (capture time, which Immich
  matches against `asset.fileCreatedAt`). Do not switch them to
  `createdAfter`/`createdBefore`, which filter on upload time and would make Recent
  and Memories windows wrong.
- `ProcessPeople` applies no date filter at all — the half-life weight is the only
  recency influence, which is why its default (3650) is much larger than the others.
  `ProcessRecent` is the same shape: the query bounds the window, so the selector
  only weights.
- `CalculateDaysDistance` normalizes both times to UTC midnight, so it is a whole-day
  difference. The future-asset guard for Recent lives in the query (`SearchAssetsByDateRange`
  passes `now` as `takenBefore`), not in the selector.

## Conventions

- Module path is `immich-album-generator`; import internal packages with that
  prefix (`immich-album-generator/pkg/selector`).
- Standard library plus only `spf13/cobra` and `joho/godotenv`. Avoid adding
  dependencies; use `net/http` and `encoding/json` directly.
- Package layout is layer-based under `pkg/`: `immich` = API access, `selector` =
  pure algorithms, `sync` = orchestration. `cmd` holds only cobra wiring.
- All logging is `log.Printf` / `log.Println` / `fmt.Print*` to the standard logger —
  no logging framework, no structured fields. Existing log lines are prefixed with the album
  type (`Recent:`, `Memories:`, `People:`) and dry-run lines with `[DRY RUN]`;
  match that style.
- Errors are wrapped with `fmt.Errorf("...: %w", err)` at the layer boundary.
- Tests are table-free, plain `testing` functions; float comparisons use
  `math.Abs(...) > 1e-9` (see `pkg/selector/*_test.go`). There is no interface or
  mock around `*immich.Client`, but it is tested end-to-end by pointing `NewClient`
  at an `httptest.NewServer` and inspecting request bodies and verbose output
  (`pkg/immich/api_test.go`, `album_test.go`, `client_test.go`). `pkg/sync` remains
  untested because it needs a live server.
- Structs in `pkg/immich/types.go` carry JSON tags matching the Immich wire format;
  types are request/response shaped rather than domain shaped.