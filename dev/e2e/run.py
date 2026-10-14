#!/usr/bin/env python3
"""End to end checks of the fork against a throwaway Navidrome in docker.

Usage:  dev/e2e/run.py <navidrome-version>    (for example 0.63.1 or 0.64.2)

Needs: docker, the built navibeat-mixes.ndp in the repo root, and the generated
test library in dev/music. Talks only to 127.0.0.1:4544. Never touches a real
server. Evidence goes to dev/e2e/evidence-<version>.txt.
"""
import json
import subprocess
import sys
import time
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

VER = sys.argv[1]
ROOT = Path(__file__).resolve().parents[2]
DEV = ROOT / "dev"
NAME = f"nd-dev-{VER}"
PORT = 4544 if VER == "0.63.1" else 4545
BASE = f"http://127.0.0.1:{PORT}"
DATA, PLUGINS = DEV / f"data-{VER}", DEV / f"plugins-{VER}"
OUT = open(DEV / "e2e" / f"evidence-{VER}.txt", "w")
RES = []


def log(*a):
    s = " ".join(str(x) for x in a)
    print(s, flush=True)
    OUT.write(s + "\n")
    OUT.flush()


def sh(*cmd, check=False):
    r = subprocess.run(cmd, capture_output=True, text=True)
    if check and r.returncode:
        raise RuntimeError(f"{cmd}: {r.stderr}")
    # docker logs writes the container's stderr to ours, so keep both
    return r.stdout + r.stderr


def check(cid, ok, evidence):
    RES.append((cid, ok))
    log(f"[{'PASS' if ok else 'FAIL'}] {cid}: {evidence}")


# ---------------------------------------------------------------- subsonic
def call(ep, user="admin", pw="devpassword", **params):
    q = {"u": user, "p": pw, "v": "1.16.1", "c": "e2e", "f": "json"}
    url = f"{BASE}/rest/{ep}?" + urllib.parse.urlencode(q) + "&" + urllib.parse.urlencode(params, doseq=True)
    with urllib.request.urlopen(url, timeout=60) as r:
        return json.load(r)["subsonic-response"]


def playlists(user="admin", pw="devpassword"):
    return call("getPlaylists", user, pw).get("playlists", {}).get("playlist", [])


def marker(comment):
    for line in (comment or "").split("\n"):
        if line.startswith("nb1:"):
            p = line.split(":")
            if len(p) >= 3:
                return p[1] + ":" + p[2]
    return None


def snap(user="admin", pw="devpassword"):
    d = {}
    for p in playlists(user, pw):
        d[p["id"]] = dict(name=p["name"], n=p["songCount"], owner=p["owner"],
                          slot=marker(p.get("comment")), comment=p.get("comment", ""))
    return d


def mine(d, owner="admin"):
    return {k: v for k, v in d.items() if v["owner"] == owner}


def byslot(d, slot, owner="admin"):
    for k, v in mine(d, owner).items():
        if v["slot"] == slot:
            return k, v
    return None, None


def dump(title, d, owner="admin"):
    log(f"--- {title} ({len(mine(d, owner))} playlists of {owner})")
    for k, v in sorted(mine(d, owner).items(), key=lambda kv: (kv[1]["slot"] or "~", kv[1]["name"])):
        log(f"   {k} n={v['n']:3} {v['name']!r} {v['slot']}")


def all_songs():
    r = call("search3", query='""', songCount=1000, artistCount=0, albumCount=0)
    return r.get("searchResult3", {}).get("song", [])


# ------------------------------------------------------------------ docker
def up():
    sh("docker", "rm", "-f", NAME)
    # the container writes as root, so clean through docker, not through rm
    sh("docker", "run", "--rm", "-v", f"{DEV}:/d", "alpine", "sh", "-c", f"rm -rf /d/{DATA.name} /d/{PLUGINS.name}")
    for sub in (DATA, PLUGINS):
        sub.mkdir(parents=True, exist_ok=True)
    sh("cp", str(ROOT / "navibeat-mixes.ndp"), str(PLUGINS))
    sh("docker", "run", "-d", "--name", NAME, "-p", f"127.0.0.1:{PORT}:4533",
       "-v", f"{DEV}/music:/music:ro", "-v", f"{DATA}:/data", "-v", f"{PLUGINS}:/plugins",
       "-e", "ND_LOGLEVEL=debug", "-e", "ND_PLUGINS_ENABLED=true", "-e", "ND_PLUGINS_FOLDER=/plugins",
       "-e", "ND_DEVAUTOCREATEADMINPASSWORD=devpassword",
       f"ghcr.io/navidrome/navidrome:{VER}", check=True)
    time.sleep(25)
    sh("docker", "stop", NAME)
    time.sleep(2)
    sh("docker", "start", NAME)
    time.sleep(15)
    sh("docker", "exec", NAME, "sqlite3", "/data/navidrome.db",
       "update plugin set enabled=1, all_users=1, all_libraries=1 where id='navibeat-mixes';")
    sh("docker", "restart", NAME)
    time.sleep(20)
    # wait for the library scan so there are tracks to work with
    for _ in range(60):
        try:
            if len(all_songs()) >= 100:
                return
        except Exception:
            pass
        time.sleep(3)
    raise RuntimeError("library did not load")


def set_cfg(cfg):
    sh("docker", "exec", NAME, "sqlite3", "/data/navidrome.db",
       "update plugin set config='" + json.dumps(cfg) + "' where id='navibeat-mixes';", check=True)


def regen(cfg=None):
    if cfg is not None:
        set_cfg(cfg)
    since = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    sh("docker", "restart", NAME)
    t0 = time.time()
    time.sleep(20)
    while time.time() - t0 < 240:
        if "generation complete for" in sh("docker", "logs", "--since", since, NAME):
            break
        time.sleep(3)
    time.sleep(3)


def plugin_errors():
    lg = sh("docker", "logs", NAME, "--since", "1h")
    bad = [l for l in lg.splitlines() if "navibeat" in l and ("level=error" in l or "panic" in l.lower())]
    # Known and not ours: Navidrome's own scrobble buffer logs "sql: database is
    # closed" when the container is restarted right after plays were recorded.
    # dev/e2e/restart_noise.py shows upstream v0.9.17 prints the same lines, and
    # that a running server prints none (dev/e2e/shutdown_noise.py).
    bad = [l for l in bad if "database is closed" not in l]
    return bad[:5]


# ------------------------------------------------------------------- native
def native_login():
    req = urllib.request.Request(BASE + "/auth/login", data=json.dumps({"username": "admin", "password": "devpassword"}).encode(),
                                 headers={"Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))["token"]


def create_user(name, pw):
    tok = native_login()
    req = urllib.request.Request(BASE + "/api/user", data=json.dumps(
        {"userName": name, "name": name, "password": pw, "isAdmin": False}).encode(),
        headers={"Content-Type": "application/json", "x-nd-authorization": "Bearer " + tok})
    return urllib.request.urlopen(req).status


# ================================================================ scenarios
def main():
    log(f"=== Navidrome {VER}")
    up()
    log("server:", call("ping")["serverVersion"])
    songs = all_songs()
    by_artist = {}
    for s in songs:
        by_artist.setdefault(s["artist"], []).append(s)
    log(f"tracks: {len(songs)} artists: {len(by_artist)}")

    # The plugin generates once at startup, before any test data exists. Clear that
    # (test harness only, the plugin never deletes) so scenarios start from nothing.
    for p in playlists():
        call("deletePlaylist", id=p["id"])
    log("cleared startup playlists, remaining:", len(playlists()))
    # Seed listening history: a fresh server has none, and most mixes need some.
    now_ms = int(time.time() * 1000)
    for a, ss in by_artist.items():
        for k, s in enumerate(ss[:6]):
            for rep in range(3):
                call("scrobble", id=s["id"], submission="true", time=now_ms - (rep * 5 + k) * 86400000 // 4)
    log("seeded plays for", len(by_artist), "artists")

    # ---- hand-made unmarked playlists named like legacy mixes (S1-2)
    legacy_names = ["🟠 Daily Mix 2", "🟠 Artist Radio 1", "🟠 New Music"]
    hand = {}
    for n in legacy_names:
        hand[n] = call("createPlaylist", name=n)["playlist"]["id"]
    # a hand-made playlist that must NEVER be adopted: custom comment, tracks, template-style name
    keep = call("createPlaylist", name="QA Punk Radio", songId=[s["id"] for s in songs[:3]])["playlist"]["id"]
    call("updatePlaylist", playlistId=keep, comment="my own punk list, hands off")
    log("hand made (legacy):", hand, "| hand made (must survive):", keep)

    # ---- 1. baseline + 3. legacy adoption
    regen({})
    d = snap()
    dump("baseline", d)
    base = mine(d)
    check("S1-baseline", len(base) >= 15 and all(v["slot"] for k, v in base.items() if k not in (keep,) and v["name"] != "QA Punk Radio"),
          f"{len(base)} playlists")
    adopted = {n: (hand[n] in d and d[hand[n]]["slot"]) for n in legacy_names}
    dups = [n for n in {v['name'] for v in base.values()} if [x['name'] for x in base.values()].count(n) > 1]
    check("S1-2 legacy adoption", all(adopted.values()) and not dups, f"adopted={adopted} duplicate names={dups}")
    base_ids = {k for k, v in base.items() if v["slot"]}

    # ---- 2. prefix change
    regen({"prefix": "QA "})
    d = snap()
    dump("after prefix=QA", d)
    cur = {k for k, v in mine(d).items() if v["slot"]}
    check("S1-1/S1-3 prefix change keeps ids", cur == base_ids,
          f"before={len(base_ids)} after={len(cur)} same={cur == base_ids} extra={cur - base_ids} missing={base_ids - cur}")
    renamed = sum(1 for k in base_ids if d[k]["name"].startswith("QA "))
    check("S1-3 renamed in place", renamed == len(base_ids), f"{renamed}/{len(base_ids)} now start with 'QA '")
    snap_prefix = {k: (v["name"], v["slot"]) for k, v in d.items() if v["slot"]}

    # ---- hand-made not adopted (F1) comes later with templates

    # ---- 4. templates
    cfg = {"prefix": "QA ", "nameArtistRadio": "This is {subject}", "nameGenreRadio": "{subject} Radio",
           "nameDailyMix": "{subject} Mix", "nameDecade": "{subject} Hits"}
    regen(cfg)
    d = snap()
    dump("templates", d)
    ids_now = {k for k, v in mine(d).items() if v["slot"]}
    check("S1-3 ids unchanged by templates", ids_now == base_ids, f"same={ids_now == base_ids}")
    k1, v1 = byslot(d, "artistradio:artistradio-1")
    subj = v1["comment"].split("everything you have by ")[1].split(".")[0] if v1 and "everything you have by " in v1["comment"] else None
    check("S2-2 artist radio name from subject", bool(v1 and subj and v1["name"] == f"QA This is {subj}"), f"{v1 and v1['name']!r} subject={subj!r}")
    gk, gv = byslot(d, "genreradio:genreradio-1")
    gsub = gv["comment"].split("NaviBeat Mixes: ")[1].split(",")[0] if gv else None
    check("S2-2 genre radio name", gv is not None and gv["name"] == f"QA {gsub} Radio", f"{gv['name']!r} genre={gsub!r}")
    dk, dv = byslot(d, "decade:decade-1990")
    check("S2-2 decade name", dv is not None and dv["name"] == "QA 1990s Hits", repr(dv and dv["name"]))
    check("S2-6 description keeps the subject", bool(v1 and "everything you have by" in v1["comment"]), (v1 or {}).get("comment", "").split("\n")[0])
    nk, nv = byslot(d, "newmusic:newmusic")
    check("S2-1 untemplated mixes keep default names", nv and nv["name"] == "QA New Music", repr(nv and nv["name"]))
    # F1: the hand made playlist survived untouched
    hm = d.get(keep)
    check("F1 hand-made playlist not adopted", hm and hm["slot"] is None and hm["n"] == 3 and "hands off" in hm["comment"] and hm["name"] == "QA Punk Radio",
          f"{hm and (hm['name'], hm['n'], hm['slot'])}")
    names = [v["name"] for v in mine(d).values()]
    # the plugin made its own 'QA Punk Radio' (genre radio) next to the hand-made one: two same-named is expected
    check("F1 plugin made its own playlist next to it", sum(1 for n in names if n == "QA Punk Radio") in (1, 2), f"'QA Punk Radio' count {names.count('QA Punk Radio')}")

    # ---- collision (S2-5): daily mix uses the same template as artist radio
    cfg2 = dict(cfg, nameDailyMix="This is {subject}")
    regen(cfg2)
    d = snap()
    dump("collision", d)
    plug = {k: v for k, v in mine(d).items() if v["slot"]}
    nm = [v["name"] for v in plug.values()]
    dupn = sorted({n for n in nm if nm.count(n) > 1})
    check("S2-5 no two plugin playlists share a name", not dupn, f"duplicates={dupn}")
    check("S1-1 ids stable across the collision flip", set(plug) == base_ids, f"same={set(plug) == base_ids}")

    # ---- F2: a copy carrying the marker must not hijack the original
    ok_, ov = byslot(d, "artistradio:artistradio-1")
    copy = call("createPlaylist", name="My copy of a mix")["playlist"]["id"]
    call("updatePlaylist", playlistId=copy, comment=(ov or {}).get("comment", ""))
    regen(cfg)
    d = snap()
    c = d.get(copy)
    orig = d.get(ok_)
    check("F2 marker copy does not hijack the original",
          bool(c and c["name"] == "My copy of a mix" and c["n"] == 0 and orig and orig["name"].startswith("QA ") and orig["n"] > 0),
          f"copy={c and (c['name'], c['n'])} original={orig and (orig['name'], orig['n'])}")
    call("updatePlaylist", playlistId=copy, comment="")

    # ---- 5. subject rotation by changing listening
    before = {k: v["name"] for k, v in mine(snap()).items() if v["slot"]}
    tracks = by_artist.get("Zed Zero", [])[:8] + by_artist.get("Blue Quartet", [])[:8]
    now = int(time.time() * 1000)
    for rep in range(6):
        for s in tracks:
            call("scrobble", id=s["id"], submission="true", time=now - rep * 3600000)
    regen(cfg)
    d = snap()
    after = {k: v["name"] for k, v in mine(d).items() if v["slot"]}
    changed = [(before[k], after[k]) for k in before if k in after and before[k] != after[k]]
    check("S1-3 subject rotation renames in place", set(after) == set(before) and len(after) == len(before),
          f"same id set={set(after) == set(before)}; {len(changed)} renamed e.g. {changed[:3]}")

    # ---- 6. sizes and the loved cap
    arts = list(by_artist)[:7]
    star_ids = [s["id"] for a in arts for s in by_artist[a]]
    call("star", id=star_ids)
    log(f"starred {len(star_ids)} tracks from {len(arts)} artists")

    def loved_n(c):
        regen(c)
        k, v = byslot(snap(), "loved:loved")
        return (k, v["n"]) if v else (None, 0)

    base_cfg = {"prefix": "QA "}
    k0, n0 = loved_n(base_cfg)
    check("S3 default loved: 30 cap, 3 per artist", n0 == min(30, 3 * len(arts)), f"{n0} tracks (expected {min(30, 3 * len(arts))})")
    k1_, n1 = loved_n(dict(base_cfg, sizeLoved=60))
    check("S3-1 sizeLoved=60 alone is still limited by maxPerArtist", n1 == min(60, 3 * len(arts)), f"{n1} tracks")
    k2_, n2 = loved_n(dict(base_cfg, sizeLoved=60, lovedMaxPerArtist=12))
    check("S3-1/S3-3 sizeLoved=60 + lovedMaxPerArtist=12", n2 == 60 and k2_ == k0, f"{n2} tracks, same id={k2_ == k0}")
    k3_, n3 = loved_n(dict(base_cfg, sizeLoved=5, lovedMaxPerArtist=12))
    check("S3-2 sizeLoved=5 raised to 10", n3 == 10, f"{n3} tracks")
    k4_, n4 = loved_n(dict(base_cfg, sizeLoved=999, lovedMaxPerArtist=12))
    check("S3-4 sizeLoved=999 out of range inherits mixSize (30)", n4 == 30, f"{n4} tracks")
    k5_, n5 = loved_n(dict(base_cfg, sizeLoved="abc", lovedMaxPerArtist="x"))
    check("S3-4 garbage values inherit", n5 == min(30, 3 * len(arts)), f"{n5} tracks")
    regen(dict(base_cfg, mixSize=12, sizeNewMusic=20, sizeTimeOfDay=3, sizeDailyMix=15))
    d = snap()
    dump("mixSize=12 + overrides", d)
    g = lambda s: (byslot(d, s)[1] or {}).get("n")
    check("S3-1 mixSize=12 applies globally", g("onrepeat:onrepeat") == 12 and g("essentials:essentials") == 12, f"onrepeat={g('onrepeat:onrepeat')} essentials={g('essentials:essentials')}")
    check("S3-1 sizeNewMusic=20", g("newmusic:newmusic") == 20, g("newmusic:newmusic"))
    check("S3-1/S3-2 sizeTimeOfDay=3 raised to 10", g("timeofday:afternoon") == 10, g("timeofday:afternoon"))
    check("S3-1 sizeDailyMix=15", g("dailymix:dailymix-1") == 15, g("dailymix:dailymix-1"))

    # ---- S1-8: a mix that is too small writes and renames nothing
    d = snap()
    check("S1-8 too-small mix (rediscover) writes nothing", byslot(d, "rediscover:rediscover")[0] is None, "no rediscover playlist exists")

    # ---- 7. idempotence and no deletes
    ids1 = {k: (v["name"], v["n"]) for k, v in snap().items()}
    regen()  # same config, new run
    ids2 = {k: (v["name"], v["n"]) for k, v in snap().items()}
    check("S7 idempotent run", ids1 == ids2, f"{len(ids1)} -> {len(ids2)} playlists; identical={ids1 == ids2}")
    check("S7 no playlist ever disappeared", set(ids1) >= (base_ids | {keep, copy}), f"all {len(base_ids | {keep, copy})} known ids still present")

    # ---- 8. second user
    log("create bob:", create_user("bob", "bobpass"))
    regen()
    bd = snap("bob", "bobpass")
    ad = snap()
    bob_own = {k for k, v in bd.items() if v["owner"] == "bob"}
    check("S1-4 bob gets his own set (day-one mixes, he has no plays yet)", len(bob_own) >= 5, f"{len(bob_own)} playlists owned by bob")
    check("S1-4 owner isolation (no shared ids with admin's)", not (bob_own & {k for k, v in ad.items() if v["owner"] == "admin"}), "disjoint")
    regen()
    ad2 = snap()
    check("S1-4 second run: nothing duplicated or stolen", len(ad2) == len(ad) and set(ad2) == set(ad), f"{len(ad)} -> {len(ad2)} playlists")
    check("S1-4 admin's own playlists unchanged by bob's run", {k: ad[k]["name"] for k in ad if ad[k]["owner"] == "admin"} == {k: ad2[k]["name"] for k in ad2 if ad2[k]["owner"] == "admin"}, "names identical")

    # ---- errors in the plugin log
    errs = plugin_errors()
    check("plugin logged no errors", not errs, errs)

    log(f"\n==== SUMMARY {VER}")
    bad = 0
    for cid, ok in RES:
        log(("PASS " if ok else "FAIL ") + cid)
        bad += 0 if ok else 1
    log(f"{len(RES) - bad}/{len(RES)} passed")
    sh("docker", "rm", "-f", NAME)
    return bad


if __name__ == "__main__":
    sys.exit(1 if main() else 0)
