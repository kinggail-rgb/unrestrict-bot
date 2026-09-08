# unrestrict-bot

Save media from restricted / private Telegram chats.

A user account (via MTProto, so it can see content that bots and content-protected
channels won't hand out) downloads the media behind a `t.me/...` link and re-posts
it somewhere you can save it.

## How it works

The service runs one gotd/td client logged in as **your user account** and watches
a **control chat** — by default your **Saved Messages**. Send it:

- a **message link** (`https://t.me/channel/123`, `https://t.me/c/123456/789`) — it
  downloads the media and posts it back into the same chat.
- a **private group invite link** (`https://t.me/+abc…`, `https://t.me/joinchat/abc…`)
  — the account joins the group (archived + muted) so it can read future links you
  send from there.

There is no bot, no Bot API server and no upload size limit (MTProto user uploads
go up to 2 GB, 4 GB with Premium). Parallel up/download is built into gotd.

### Optional bot mode

Set `BOT_TOKEN` and the service _also_ runs the classic "DM a bot a link and get
the media back" flow, sharing the same download pipeline.

## Improvements over the Python version

- Single static Go binary; the `telegram-bot-api` sidecar container is gone.
- **Session stored in the database**, not an env var — `run` logs you in on
  first start and persists the session in the `/data` volume.
- **Dedup cache** (embedded bbolt): re-sending a link already handled in the
  control chat is served instantly by re-forwarding the earlier result.
- **HTTP health + metrics**: `GET /healthz`, `GET /metrics` (expvar). The
  `/tmp/healthz` file is still touched for backwards compatibility.
- Structured logging (`log/slog`, `LOG_FORMAT=text|json`) and fail-fast config
  validation.
- Centralised `FLOOD_WAIT` handling and graceful shutdown on `SIGTERM`.
- Persistent peer cache, so private-channel access survives restarts.

## Configuration

All configuration is environment variables.

| Variable                     | Required        | Default          | Notes                                                                                                                     |
| ---------------------------- | --------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `API_ID`, `API_HASH`         | yes             | –                | from <https://my.telegram.org>                                                                                            |
| `SESSION_STRING`             | no              | –                | optional seed, imported on first run; a Telethon `STRING_SESSION` also works. Normally you just run `gen-session` instead |
| `CONTROL_CHAT`               | no              | `me`             | `me` (Saved Messages), an `@username`, or a numeric chat id like `-1001234567890`                                         |
| `BOT_TOKEN`                  | no              | –                | set to also enable bot DM mode                                                                                            |
| `TEMP_DIR`                   | no              | OS temp dir      | scratch space for downloads                                                                                               |
| `CACHE_PATH`                 | no              | `/data/cache.db` | bbolt database (session + peer store + dedup cache + forward progress)                                                    |
| `HEALTH_ADDR`                | no              | `:8080`          | health / metrics listener                                                                                                 |
| `HEALTH_FILE`                | no              | `/tmp/healthz`   | touched for legacy healthchecks                                                                                           |
| `LOG_LEVEL`                  | no              | `info`           | `debug` \| `info` \| `warn` \| `error`                                                                                    |
| `LOG_FORMAT`                 | no              | `text`           | `text` \| `json`                                                                                                          |
| `FORWARD_FROM`, `FORWARD_TO` | yes (`forward`) | –                | bulk-forward source / destination                                                                                         |

## Running

```sh
export API_ID=... API_HASH=...
go run . run          # or: go build -o unrestrict-bot . && ./unrestrict-bot
```

On the first start, if no session is stored and stdin is a terminal, it prompts
for your phone number, the login code, and your 2FA password (if any), saves the
session into the database (`CACHE_PATH`), and carries on. `forward` does the same.
Later starts need no interaction.

Then, from your Saved Messages, send a `t.me` link.

An existing Telethon `STRING_SESSION` (or a native `SESSION_STRING`) works as a
seed — set it once and it is imported on first run instead of prompting.

### Docker

```sh
cp .env.example .env                              # API_ID / API_HASH
podman compose run --rm -it telegram-bot run      # first start: prompts, saves to /data
# Ctrl-C once it says "user client ready", then:
podman compose up --build -d
```

`gen-session` does the same login but only prints a portable `SESSION_STRING` and
exits — use it when you want to move the session to another host.

The compose file uses `network_mode: "service:gluetun"` and bundles a `gluetun`
service; drop it and adjust if you are not routing through a VPN.

The compose file defines a healthcheck. Podman without systemd (e.g. Artix) never
runs it on a schedule, so `podman ps` shows `(starting)` indefinitely — that is
cosmetic. `podman healthcheck run telegram-bot` runs the probe on demand, or hit
`GET /healthz` yourself (publish port `8080` on the `gluetun` service first).

## Bulk forward

Copy every photo/video from one chat to another, oldest first, resuming from
saved progress:

```sh
export FORWARD_FROM=@source_channel FORWARD_TO=-1001234567890
go run . forward
```

## Subcommands

| Command                      | Purpose                                                                  |
| ---------------------------- | ------------------------------------------------------------------------ |
| `unrestrict-bot run`         | start the service; logs in on first start if needed (default)            |
| `unrestrict-bot gen-session` | log in and print a portable `SESSION_STRING`, then exit                  |
| `unrestrict-bot forward`     | one-shot bulk media forward                                              |
| `unrestrict-bot healthcheck` | probe local `/healthz`, exit non-zero if unhealthy (compose healthcheck) |

## Development

```sh
go test ./...
go vet ./...
```

## Disclaimer

This software was originally written in Python, it had some not so well implemented hacks to speed up downloads, bypass limits and flood, etc. I used Claude to rewrite the software in Go and made some changes to how it behaves like removing telegram-bot-api dependency, improving auth flow, removing the requirement of a bot frontend and many other minor decisions to improve the QOL that this software was lacking.

While AI was used, everything was reviwed, tested and validated, some parts even manually rewriten to improve performance and fix gaps that the AI ignored. This is not a vibe coded project.
