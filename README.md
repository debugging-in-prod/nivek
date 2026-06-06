# nivek

A Go-based Twitch integration platform for live Dwarf Fortress streaming.

**nivek** lets Twitch chatters interact with my Dwarf Fortress game in real time while I keep the game running uninterrupted on stream. Chat messages are filtered and forwarded to the game PC, where they can trigger in-game actions. Meanwhile, automated rolling snapshots of the fortress are pushed to a public web dashboard so viewers can check status, citizens, and world state without me tabbing out or pausing the stream.

### Architecture

The system is built as four independent Go binaries (under `cmd/`) that communicate securely:

- **`cmd/twitch-bot`** — Production Twitch IRC bot built on `go-twitch-irc`.  
  Dynamically joins channels for all active users stored in PostgreSQL. Validates usernames, graceful shutdown, and forwards filtered commands via WebSocket to the executor.

- **`cmd/df-executor`** — Receives authenticated commands from the bot and executes them on the Dwarf Fortress host (LAN/WebSocket).

- **`cmd/df-snapshot-pusher`** — Runs as a daemon on the DF host. Every ~15 seconds it reads the latest atomic snapshot JSON produced by a DFHack Lua script (`snapshot-scan.lua`), validates it, signs it with HMAC-SHA256, gzips the payload, and POSTs it to the dashboard API. Zero inbound connections — fully outbound and secure.

- **`cmd/core-api`** — Echo-based web backend serving the public dashboard at [nivek.life/df](https://nivek.life/df). Handles snapshot ingestion, HMAC verification, and serves real-time fortress data.

### Key Technical Highlights

- **Clean multi-binary Go layout** with shared `internal/libraries/` packages
- **PostgreSQL persistence** via GORM for user/channel management
- **Secure inter-service communication** (HMAC signing + WebSockets + JWT)
- **Graceful shutdown** and signal handling on all daemons
- **Production-grade configuration** via environment variables + godotenv
- **Real-time chat processing** with filtering and command execution
- **Efficient snapshot pipeline** — atomic file reads, validation, gzip compression, and outbound HTTPS pushes
- **Live public dashboard** built with Vite showing fortress state without interrupting the stream

Currently unfinished. Intended to run 24/7 during streams with zero downtime for the game.

**Stack**: Go 1.26, Echo, GORM/Postgres, go-twitch-irc, WebSockets, HMAC-SHA256, Logrus
