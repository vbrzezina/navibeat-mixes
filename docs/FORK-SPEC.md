# Fork spec: stable identity, subject names, per-mix sizes

Status: approved scope, implementation in progress.
Base: upstream v0.9.17. Fork version: `0.9.17-vb.1`.
Rule for every file in this repo, including this one: no em dash.

## Why

Three things a real user hit on a real server:

1. Changing the playlist **prefix** created a whole second set of playlists and
   orphaned the first, because the plugin finds its playlists by exact **name**.
2. The user wants playlists named after what is in them ("This is Linkin Park",
   "Punk Radio"). Today the subject only appears in the description, because a
   name that changes would orphan the playlist (see 1).
3. `mixSize` is one global number. "Your Loved Songs" is capped at 30 although
   the user starred 81 tracks, and the global `maxPerArtist` (3) also applies to it.

## Non-goals

- No new permissions, no network access, no new dependencies.
- No deletion of playlists, ever. The fork never calls `deletePlaylist`.
- No change to which tracks a mix selects, except the size and per-artist
  limits asked for in story 3.
- Existing installs that touch no new setting must behave exactly like 0.9.17,
  apart from story 1 (which only removes duplicates).

## Story 1: identity by marker, rename in place

Every playlist the plugin writes carries a machine line in its description:
`nb1:<kind>:<slot>:<date>:<mode>:<count>`. `<kind>` plus `<slot>` is the stable
id of a mix (for example `artistradio` + `artistradio-1`).

Acceptance criteria:

- **S1-1** The plugin looks up its playlist for a mix by (owner, kind, slot)
  parsed from the description. If found, that playlist (same id) is updated.
- **S1-2** If no marker match exists, it falls back to an exact name match for
  the same owner (playlists made by older versions). When that matches, the
  playlist is adopted: tracks and description are rewritten, so it carries the
  marker from then on. No duplicate is created.
- **S1-3** If the stored name differs from the name the mix should have now
  (prefix change, template change, rotation), the playlist is renamed in place
  with `updatePlaylist` (parameter `name`). The id does not change.
- **S1-4** The owner check stays: only playlists owned by the current user are
  ever matched (admins see everyone's playlists).
- **S1-5** If several of the owner's playlists carry the same (kind, slot), the
  most recently changed one is used and the others are left untouched.
- **S1-6** A failed rename is logged and does not fail the write: tracks and
  description are still written, and the old name stays until the next run.
- **S1-7** The control mailbox playlist logic is unchanged.
- **S1-8** A mix that is "too small" (under the minimum) still writes nothing and
  renames nothing.

## Story 2: name templates with one placeholder

Each mix family has exactly one subject, so there is exactly one placeholder:
`{subject}`.

| Family | Setting | Subject | Example template | Result |
|---|---|---|---|---|
| Artist Radio | `nameArtistRadio` | the artist | `This is {subject}` | `This is Billy Talent` |
| Daily Mix | `nameDailyMix` | the anchor artist | `{subject} Mix` | `Billy Talent Mix` |
| Genre Radio | `nameGenreRadio` | the genre | `{subject} Radio` | `Punk Radio` |
| Decades | `nameDecade` | the decade label | `{subject} Hits` | `1990s Hits` |

Acceptance criteria:

- **S2-1** An empty template (the default) gives exactly today's name
  (`Artist Radio 1`, `Daily Mix 2`, `Genre Radio 3`, `1990s`).
- **S2-2** `{subject}` is replaced by the subject. Any other text is kept. The
  placeholder is case sensitive and may appear zero or more times.
- **S2-3** The rendered name is cleaned: control characters removed, runs of
  whitespace collapsed, leading and trailing whitespace trimmed, and cut to 100
  characters without splitting a multi-byte character. The configured prefix is
  added after rendering and is not counted in the 100.
- **S2-4** If the cleaned result is empty, the default name is used.
- **S2-5** Collisions: if a rendered name (prefix included) equals the name of an
  earlier mix in the same run, the later mix uses its default numbered name
  instead. Order is the plan order, so the result is deterministic.
- **S2-6** The description text is unchanged (it keeps naming the subject).
- **S2-7** The existing time-of-day and Rediscover name settings keep working.

## Story 3: per-mix size and per-artist override

Acceptance criteria:

- **S3-1** New integer settings `size<Mix>`, one per mix family: `sizeNewMusic`,
  `sizeLoved`, `sizeOnRepeat`, `sizeEssentials`, `sizeDiscovery`,
  `sizeRediscover`, `sizeWrapped`, `sizeGenreRadio`, `sizeArtistRadio`,
  `sizeDailyMix`, `sizeDecade`, `sizeTimeOfDay` (one value for all four).
- **S3-2** `0` (the default) means "use `mixSize`". A value from 1 to 9 is raised
  to 10 (the minimum a mix needs). The maximum is 200, like `mixSize`.
- **S3-3** New integer setting `lovedMaxPerArtist`. `0` (default) means "use
  `maxPerArtist`". Any positive value replaces it for Your Loved Songs only.
  Maximum 1000.
- **S3-4** Out of range or non-numeric values fall back to the default and never
  crash configuration parsing.

## Settings form and docs

- Every new setting has a title, a default, and a description in
  `manifest.json`, and appears in the matching group of the form: names in
  "Names", sizes in a new group "Sizes".
- README documents the three features, the `{subject}` placeholder with the
  table above, and the behaviour change in story 1.
- `manifest.json` version becomes `0.9.17-vb.1`.

## Test plan

- Unit tests for every acceptance criterion above (`make test`).
- `make check-emdash` and `go vet ./internal/...` clean.
- `make plugin.wasm` builds with plain Go (wasip1).
- End to end on a scratch Navidrome (`dev/up.sh`, docker): generation creates
  playlists; change the prefix and confirm no duplicates and the same ids; set a
  template and confirm a rename in place with the same id; set `sizeLoved` and
  confirm the track count.
- Independent code review before merge, then a full re-run of all checks.
