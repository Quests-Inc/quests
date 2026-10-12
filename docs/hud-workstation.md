# Workstation: the HUD against a remote API

The overlay (GTK4 + layer-shell) lives **on a PC with Wayland/niri**. The API can be local
or on a server (systemd / Docker / tailnet). Below is the typical case: the API is already running somewhere on the network and the HUD
is brought up locally through user systemd.

## Why “just a login” is not enough

| Client | How it reaches the API |
|--------|-----------------|
| Browser (SPA) | a session cookie after `POST /api/auth/login` |
| HUD / CLI / bot / MCP | `Authorization: Bearer …` from `QUESTS_API_TOKEN` |

An account's login and password are **not** put into the overlay. A token is minted in the web interface or on the
machine with the database and is shown **once** (only a hash is stored). Details:
[`auth.md`](auth.md).

Check that auth is on:

```bash
curl -sS http://SERVER:8765/api/auth/state
# {"auth_required":true,...}  →  the HUD needs a token
```

## 1. A bearer token

**From the interface (easiest):** Settings → «Подключения» (Connections) → create a “HUD” token — see [connecting-clients.md](connecting-clients.md). Below is the same from the server host.

**Docker (compose `quests-api`):**

```bash
ssh USER@SERVER
docker exec quests-api quests-server token ls
docker exec quests-api quests-server token add overlay-pc
# save the line QUESTS_API_TOKEN=… — it will not be shown a second time
```

**systemd / a binary on the host:**

```bash
quests-server token add overlay-pc
```

An old `overlay` token in `token ls` without its secret is useless — either a new
`token add …`, or `token rm <id>` and start over.

## 2. Env on the PC

```bash
mkdir -p ~/.config/quests
umask 077
cat > ~/.config/quests/overlay.env <<'EOF'
QUESTS_API=http://SERVER:8765
QUESTS_WEB_URL=http://SERVER:8765
QUESTS_API_TOKEN=<secret>
EOF
chmod 600 ~/.config/quests/overlay.env
```

`api_base` in `data/overlay.json` is optional (the env wins over the file). Do not put the token
in the json.

Check before starting the HUD:

```bash
set -a && source ~/.config/quests/overlay.env && set +a
curl -sS -H "Authorization: Bearer $QUESTS_API_TOKEN" "$QUESTS_API/api/health"
```

## 3. The repo and the systemd unit

The units expect the tree in `~/Quests`:

```bash
# if the clone lives elsewhere:
ln -sfn /path/to/quests ~/Quests

mkdir -p ~/.config/systemd/user/quests-overlay.service.d
ln -sfn ~/Quests/deploy/systemd/user/quests-overlay.service \
  ~/.config/systemd/user/quests-overlay.service

cat > ~/.config/systemd/user/quests-overlay.service.d/override.conf <<'EOF'
[Service]
EnvironmentFile=-%h/.config/quests/overlay.env
EOF
```

Overlay dependencies (Arch/CachyOS): `gtk4`, `gtk4-layer-shell`, `python-gobject`.

## 4. Starting

In a graphical session (otherwise there is no `WAYLAND_DISPLAY`):

```bash
systemctl --user import-environment WAYLAND_DISPLAY XDG_RUNTIME_DIR DISPLAY NIRI_SOCKET
systemctl --user daemon-reload
systemctl --user enable --now quests-overlay.service
systemctl --user status quests-overlay.service
```

Expected: `components.overlay.status == "ok"` in `/api/health` (the HUD chip in the SPA).

A one-off smoke run without systemd: `just run-overlay` with the same env —
for debugging. For permanent operation use the unit only.

## 5. niri binds

IPC: `python3 -m overlay toggle|monitor|status` (the socket is
`$XDG_RUNTIME_DIR/quests-overlay.sock`). Run it from the repo root:

```kdl
// ~/.config/niri/config.kdl
Mod+Space hotkey-overlay-title="Quests HUD: toggle" {
    spawn-sh "cd ~/Quests && python3 -m overlay toggle"
}
Mod+Shift+Space hotkey-overlay-title="Quests HUD: next monitor" {
    spawn-sh "cd ~/Quests && python3 -m overlay monitor"
}

layer-rule {
    match namespace="^quests-"
}
```

## Checklist: why it does not come up

| Symptom | What to check |
|---------|----------------|
| `401` / an empty HUD | `QUESTS_API_TOKEN` is missing / expired; auth is required |
| unit inactive, GTK errors | the Wayland `import-environment` was not done |
| `Overlay socket not found` | the daemon is not running (`quests-overlay.service`) |
| the unit fails on `~/Quests/...` | no symlink / a wrong `WorkingDirectory` |
| heartbeat offline | the token and API are fine in curl, but the env is not in the unit (no drop-in) |
| the niri bind does nothing | `cd` into a path that does not exist in `spawn-sh` |

## Related

- Deploying the API to a server: [`../deploy/SERVER.md`](../deploy/SERVER.md)
- User units: [`../deploy/systemd/README.md`](../deploy/systemd/README.md)
- Auth / tokens: [`auth.md`](auth.md)
