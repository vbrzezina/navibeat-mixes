#!/usr/bin/env python3
"""Control experiment: are the 'database is closed' scrobble-buffer errors caused by
restarting right after scrobbling, regardless of plugin build?
Run: dev/e2e/restart_noise.py <navidrome-version> <path-to.ndp> <label>"""
import json, subprocess, sys, time, urllib.parse, urllib.request
from pathlib import Path
VER, NDP, LABEL = sys.argv[1], sys.argv[2], sys.argv[3]
ROOT = Path(__file__).resolve().parents[2]; DEV = ROOT / "dev"
NAME, PORT = f"nd-rn-{LABEL}", 4561 if LABEL == "upstream" else 4562
D, P = DEV / f"data-rn-{LABEL}", DEV / f"plugins-rn-{LABEL}"
def sh(*c): r = subprocess.run(c, capture_output=True, text=True); return r.stdout + r.stderr
def call(ep, **k):
    q = {"u": "admin", "p": "devpassword", "v": "1.16.1", "c": "n", "f": "json"}
    u = f"http://127.0.0.1:{PORT}/rest/{ep}?" + urllib.parse.urlencode(q) + "&" + urllib.parse.urlencode(k, doseq=True)
    return json.load(urllib.request.urlopen(u, timeout=60))["subsonic-response"]
sh("docker", "rm", "-f", NAME)
sh("docker", "run", "--rm", "-v", f"{DEV}:/d", "alpine", "sh", "-c", f"rm -rf /d/{D.name} /d/{P.name}")
D.mkdir(exist_ok=True); P.mkdir(exist_ok=True); sh("cp", NDP, str(P / "navibeat-mixes.ndp"))
sh("docker", "run", "-d", "--name", NAME, "-p", f"127.0.0.1:{PORT}:4533", "-v", f"{DEV}/music:/music:ro",
   "-v", f"{D}:/data", "-v", f"{P}:/plugins", "-e", "ND_LOGLEVEL=debug", "-e", "ND_PLUGINS_ENABLED=true",
   "-e", "ND_PLUGINS_FOLDER=/plugins", "-e", "ND_DEVAUTOCREATEADMINPASSWORD=devpassword", f"ghcr.io/navidrome/navidrome:{VER}")
time.sleep(25); sh("docker", "stop", NAME); time.sleep(2); sh("docker", "start", NAME); time.sleep(15)
sh("docker", "exec", NAME, "sqlite3", "/data/navidrome.db", "update plugin set enabled=1, all_users=1, all_libraries=1 where id='navibeat-mixes';")
sh("docker", "restart", NAME); time.sleep(25)
songs = call("search3", query='""', songCount=1000, artistCount=0, albumCount=0)["searchResult3"]["song"]
mark = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
for rnd in range(3):
    for s in songs[rnd * 50: rnd * 50 + 50]:
        call("scrobble", id=s["id"], submission="true")
    sh("docker", "restart", NAME)       # restart immediately, like the e2e driver does
    time.sleep(30)
lg = sh("docker", "logs", "--since", mark, NAME).splitlines()
bad = [l for l in lg if "level=error" in l and "database is closed" in l]
other = [l for l in lg if "level=error" in l and "database is closed" not in l and "navibeat" in l]
print(f"[{LABEL}] 3 x (50 scrobbles then immediate restart): 'database is closed' errors={len(bad)}, other plugin errors={len(other)}")
for l in (bad[:1] + other[:2]): print("    ", l[:190])
sh("docker", "rm", "-f", NAME)
