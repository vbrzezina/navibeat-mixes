#!/usr/bin/env python3
"""Do the 'database is closed' scrobble-buffer errors come from a running server
or only from shutdown? Run: dev/e2e/shutdown_noise.py <navidrome-version>"""
import json, subprocess, sys, time, urllib.parse, urllib.request
from pathlib import Path
VER = sys.argv[1]
ROOT = Path(__file__).resolve().parents[2]; DEV = ROOT / "dev"
NAME, PORT = f"nd-noise-{VER}", 4560
D, P = DEV / f"data-noise-{VER}", DEV / f"plugins-noise-{VER}"
def sh(*c): r = subprocess.run(c, capture_output=True, text=True); return r.stdout + r.stderr
def call(ep, **k):
    q = {"u": "admin", "p": "devpassword", "v": "1.16.1", "c": "n", "f": "json"}
    u = f"http://127.0.0.1:{PORT}/rest/{ep}?" + urllib.parse.urlencode(q) + "&" + urllib.parse.urlencode(k, doseq=True)
    return json.load(urllib.request.urlopen(u, timeout=60))["subsonic-response"]
def errs(since):
    return [l for l in sh("docker", "logs", "--since", since, NAME).splitlines() if "level=error" in l and ("navibeat" in l or "scrobble" in l)]
sh("docker", "rm", "-f", NAME)
sh("docker", "run", "--rm", "-v", f"{DEV}:/d", "alpine", "sh", "-c", f"rm -rf /d/{D.name} /d/{P.name}")
D.mkdir(exist_ok=True); P.mkdir(exist_ok=True); sh("cp", str(ROOT / "navibeat-mixes.ndp"), str(P))
sh("docker", "run", "-d", "--name", NAME, "-p", f"127.0.0.1:{PORT}:4533", "-v", f"{DEV}/music:/music:ro",
   "-v", f"{D}:/data", "-v", f"{P}:/plugins", "-e", "ND_LOGLEVEL=debug", "-e", "ND_PLUGINS_ENABLED=true",
   "-e", "ND_PLUGINS_FOLDER=/plugins", "-e", "ND_DEVAUTOCREATEADMINPASSWORD=devpassword", f"ghcr.io/navidrome/navidrome:{VER}")
time.sleep(25); sh("docker", "stop", NAME); time.sleep(2); sh("docker", "start", NAME); time.sleep(15)
sh("docker", "exec", NAME, "sqlite3", "/data/navidrome.db", "update plugin set enabled=1, all_users=1, all_libraries=1 where id='navibeat-mixes';")
sh("docker", "restart", NAME); time.sleep(25)
songs = call("search3", query='""', songCount=1000, artistCount=0, albumCount=0)["searchResult3"]["song"]
mark = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
for s in songs[:40]:
    call("scrobble", id=s["id"], submission="true")
time.sleep(45)
running = errs(mark)
print(f"[{VER}] scrobbled 40 plays, waited 45s with the server RUNNING: {len(running)} error lines")
for l in running[:3]: print("   ", l[:200])
mark2 = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
for s in songs[40:80]:
    call("scrobble", id=s["id"], submission="true")
sh("docker", "stop", "-t", "1", NAME)   # abrupt-ish stop while the buffer is busy
after = errs(mark2)
print(f"[{VER}] scrobbled 40 more and stopped the container at once: {len(after)} error lines")
for l in after[:3]: print("   ", l[:200])
sh("docker", "rm", "-f", NAME)
