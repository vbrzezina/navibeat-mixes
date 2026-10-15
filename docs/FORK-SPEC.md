# Fork spec: stable identity and subject names

Status: scope cut in phase 1 (per-mix sizes dropped).
Base: upstream v0.9.17. Fork version: `0.9.17-vb.2`.
Rule for every file in this repo, including this one: no em dash.

## Why

Three things a real user hit on a real server:

1. Changing the playlist **prefix** created a whole second set of playlists and
   orphaned the first, because the plugin finds its playlists by exact **name**.
2. The user wants playlists named after what is in them ("This is Linkin Park",
   "Punk Radio"). Today the subject only appears in the description, because a
   name that changes would orphan the playlist (see 1).
3. (Dropped in phase 1.) `mixSize` is one global number; per-mix sizes were
   considered and cut. Sizes stay exactly as upstream.

## Non-goals

- No new permissions, no network access, no new dependencies.
- No deletion of playlists, ever. The fork never calls `deletePlaylist`.
- No change to which tracks a mix selects.
- Existing installs that touch no new setting must behave exactly like 0.9.17,
  apart from story 1 (which only removes duplicates).

## Story 1: identity by marker, rename in place

Every playlist the plugin writes carries a machine line in its description:
`nb1:<kind>:<slot>:<date>:<mode>:<count>`. `<kind>` plus `<slot>` is the stable
id of a mix (for example `artistradio` + `artistradio-1`).

Acceptance criteria:

- **S1-1** The plugin looks up its playlist for a mix by (owner, kind, slot)
  parsed from the description. If found, that playlist (same id) is updated.
- **S1-2** If no marker match exists, an unmarked playlist of the same owner
  is adopted (playlists made by older versions) only if it plausibly came from
  this plugin: its name is the LEGACY default name of the mix (prefix plus the
  bare default, for example `Genre Radio 1`), or its name is the wanted name
  and its description contains the attribution line. A hand made playlist that
  merely has the same name is NOT adopted and a new one is created. Adopted
  playlists have tracks and description rewritten and carry the marker from
  then on. A playlist whose marker names a different mix is never adopted.
- **S1-3** If the stored name differs from the name the mix should have now
  (prefix change, template change, rotation), the playlist is renamed in place
  with `updatePlaylist` (parameter `name`). The id does not change.
- **S1-4** The owner check stays: only playlists owned by the current user are
  ever matched (admins see everyone's playlists).
- **S1-5** If several of the owner's playlists carry the same (kind, slot)
  (a duplicate, or a copy the user made, which keeps the marker), one is chosen
  by rank: (1) its name equals the wanted name, (2) its name equals the legacy
  default name, (3) otherwise the most recently changed, then the most recently
  created, then the first listed. Ties inside a tier use the same recency
  order. The others are left untouched.
- **S1-6** A failed rename is logged and does not fail the write: tracks and
  description are still written, and the old name stays until the next run.
  A rename onto a name another of the owner's playlists already has is still
  done, and one log line names both ids.
- **S1-6a** A playlist created in this call gets its description (the marker)
  written before its tracks; if that write fails the slot counts as failed and
  is retried. An existing playlist keeps tracks first, and a failed description
  write is only logged.
- **S1-6b** A hand renamed mix is renamed back on the next run: the plugin owns
  the name.
- **S1-7** The control mailbox playlist logic is unchanged.
- **S1-8** A mix that is "too small" (under the minimum) still writes nothing and
  renames nothing.

## Story 2: name templates with one placeholder

Each mix family has exactly one subject, so there is exactly one placeholder:
`{subject}`.

| Family | Setting | Subject | Example template | Result |
|---|---|---|---|---|
| Artist Radio | `nameArtistRadio` | the artist | `This is {subject}` | `This is Billy Talent` |
| Genre Radio | `nameGenreRadio` | the genre | `{subject} Radio` | `Punk Radio` |
| Decades | `nameDecade` | the decade label | `{subject} Hits` | `1990s Hits` |

Acceptance criteria:

- **S2-1** An empty template (the default) gives exactly today's name
  (`Artist Radio 1`, `Genre Radio 3`, `1990s`).
- **S2-2** `{subject}` is replaced by the subject. Any other text is kept. The
  placeholder is case sensitive and may appear zero or more times.
- **S2-3** The rendered name is cleaned: control characters removed, runs of
  whitespace collapsed, leading and trailing whitespace trimmed, and cut to 100
  characters without splitting a multi-byte character. The configured prefix is
  added after rendering and is not counted in the 100.
- **S2-4** If the cleaned result is empty, the default name is used.
- **S2-5** Collisions: a mix with a fixed name (name equals default: Morning,
  Daily Mix N, New Music, ...) always keeps it, exactly as upstream, and its name
  is reserved first. If the name rendered from a template equals a reserved name
  or the name of an earlier templated mix in the same run, that mix uses its
  default numbered name instead; if that is taken too, " (2)", " (3)" and so on
  is appended to it. The suffix is added after the 100 character cut and the
  prefix, so it is never cut off. Order is the plan order, so the result is
  deterministic, and a rotating template can never push a fixed mix to a new name.
- **S2-6** The description text is unchanged (it keeps naming the subject). A
  subject is cleaned (naming.Subject: control characters and line breaks become
  spaces, cut to 100 characters) before it enters a name or a sentence, so a
  tag cannot inject a machine line.
- **S2-7** The existing time-of-day and Rediscover name settings keep working.
- **S2-8** Daily Mix has no name setting: it keeps its fixed name "Daily Mix N".
  Its description sentence is still cleaned (naming.Sentence) and its name is
  reserved in the collision step like any fixed name.
- **S2-9** Settings removed in phase 1 (`size*`, `lovedMaxPerArtist`,
  `nameDailyMix`) may still be saved in an install that had them; they are
  ignored without an error.

## Story 3: per-mix size and per-artist override (dropped in phase 1)

Dropped in phase 1, sizes stay as upstream: no `size*` settings and no
`lovedMaxPerArtist`. `mixSize` and `maxPerArtist` apply to every mix as before.
The ReplaceTracks long id split that existed only for big sizes is gone too.

## Settings form and docs

- Every new setting has a title, a default, and a description in
  `manifest.json`, and appears in the matching group of the form: names in
  "Names".
- README documents the two features, the `{subject}` placeholder with the
  table above, and the behaviour change in story 1.
- `manifest.json` version becomes `0.9.17-vb.2`.

## Test plan

- Unit tests for every acceptance criterion above (`make test`).
- `make check-emdash` and `go vet ./internal/...` clean.
- `make plugin.wasm` builds with plain Go (wasip1).
- End to end on a scratch Navidrome (`dev/up.sh`, docker): generation creates
  playlists; change the prefix and confirm no duplicates and the same ids; set a
  template and confirm a rename in place with the same id.
- Independent code review before merge, then a full re-run of all checks.
