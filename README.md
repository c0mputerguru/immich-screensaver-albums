# immich-album-generator

A Go CLI that queries an [Immich](https://immich.app/) server and syncs assets into
named albums for use as a photo-frame / screensaver feed. It maintains three album
types:

- **Recent** – assets captured in the last N days.
- **Memories** – assets captured around today's month/day in past years.
- **People** – assets containing given people.

When more assets match than the configured limit allows, the tool does **not** cut
off the oldest ones. It picks a subset with exponential-decay weighted random
sampling, so older assets still surface occasionally. Results differ on every run,
which is intentional for a screensaver.

The tool is designed to run repeatedly (e.g. nightly via cron): each run recomputes
the desired album contents and diffs them against the current album state. **Assets
that were not selected this run are removed from the album**, so each album always
converges to exactly the selected set.

## Build

```bash
go build -o bin/immich-album-generator .
go run . --help
```

Requires Go (module targets `go 1.26.1`). Only `spf13/cobra` and `joho/godotenv`
are used beyond the standard library.

## Credentials

`--url` / `--api-key` default to the `IMMICH_URL` / `IMMICH_API_KEY` environment
variables. A `.env` file in the working directory is loaded automatically. Both
values are required or the run aborts.

```bash
IMMICH_URL=https://immich.example.com IMMICH_API_KEY=xxxx go run . --dry-run
```

## Usage

```bash
# Preview everything without mutating the server
go run . --dry-run

# Only sync the People album for two people, skip Recent and Memories
go run . --recent-limit 0 --memories-limit 0 --people "Alice,Bob"

# Custom recent window and album name
go run . --recent-days 30 --recent-album-name "Screensaver"
```

Enable/disable logic: the Recent block runs only when `--recent-limit > 0`, Memories
only when `--memories-limit > 0`, and People only when `--people` is non-empty.
Setting a limit to `0` is the way to skip an album type.

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--url` | `$IMMICH_URL` | Immich server URL |
| `--api-key` | `$IMMICH_API_KEY` | Immich API key |
| `--recent-days` | `90` | Recent: days back from today |
| `--recent-limit` | `5000` | Recent: max assets (`0` skips the album) |
| `--recent-half-life` | `30.0` | Recent: decay half-life in days |
| `--recent-album-name` | `ScreensaverRecent` | Recent: album name |
| `--memories-days` | `14` | Memories: +/- days around today's date |
| `--memories-limit` | `5000` | Memories: max assets (`0` skips the album) |
| `--memories-half-life` | `7.0` | Memories: decay half-life in days |
| `--memories-album-name` | `ScreensaverMemories` | Memories: album name |
| `--people` | none | People names or IDs, comma separated |
| `--people-limit` | `5000` | People: max assets |
| `--people-half-life` | `3650.0` | People: decay half-life in days |
| `--people-album-name` | `ScreensaverPeople` | People: album name |
| `--dry-run` | `false` | Log intended changes without mutating the server |
| `--verbose` | `false` | Log every API request/response to stderr |

## How selection works

Each album type fetches its candidates, computes a decay weight per asset, then
chooses up to `limit` with A-Res weighted random sampling (without replacement).

- Weight: `0.5 ^ (distance / halfLife)` – recent assets weigh more, older ones decay.
- Distance for Recent/People: absolute whole-day difference from today (UTC).
- Distance for Memories: month/day comparison ignoring year, so Dec 31 vs Jan 1 is
  1 day apart, not 364.
- When `limit >= len(assets)`, every candidate is kept.

Weights are computed from `FileCreatedAt` (capture time). People applies no date
filter besides the weight, which is why its default half-life is much larger. People
has no `--people-days` flag; the decay weight is the only recency control.

## Architecture

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

- `pkg/immich` – HTTP client and Immich API access, including pagination and
  endpoint fallbacks across Immich versions.
- `pkg/selector` – pure, dependency-free weighting/distance algorithms. Its tests
  need no HTTP server; keep logic here when it can be a pure function.
- `pkg/sync` – orchestration: fetch, weight, select, ensure album, diff and apply.

## Notes

- `dryRun` is threaded through each `Sync*` call; `EnsureAlbum` returns a synthetic
  album ID and `SyncAlbumAssets` logs intended adds/removes without mutating.
- A failed album is logged and the run continues to the next one, exiting `0`
  overall. A zero exit does not guarantee every album synced.
- Recent window bounded by the search query (`takenAfter`/`takenBefore`, i.e. capture
  time); Memories performs one search per year for the past 50 years.

## Tests

```bash
go test ./...
go vet ./...
```

`pkg/selector` and `pkg/immich` are covered (`pkg/immich` uses `httptest` stubs);
`pkg/sync` has no tests because it needs a live server.
