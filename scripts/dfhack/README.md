# DFHack scripts (DFHost-side)

Lua scripts that run inside DFHack on the DFHost. They produce data the
rest of the nivek pipeline consumes — currently the map snapshot that feeds
the `/df` dashboard on peanutbudderbot.com.

## Installation on DFHost

DFHack auto-discovers scripts in `dfhack-config/scripts/` (relative to the DF
install directory). Symlink each script there so repo updates flow through
without copying. With `DF_DIR` set to your DF install directory:

```
ln -s "$PWD/scripts/dfhack/snapshot-scan.lua" \
  "$DF_DIR/dfhack-config/scripts/snapshot-scan.lua"
```

Then make it start on every fort load by appending to
`$DF_DIR/dfhack-config/init/onMapLoad.init`:

```
snapshot-scan start
```

repeat-util cancels the scan automatically on world unload, so leaving a fort
stops it. You can also control it manually at the DFHack console:

```
snapshot-scan start | stop | status
```

## Scripts

- **`snapshot-scan.lua`** — incrementally scans the current fortress's tiles,
  furniture, and citizens (the full map height, all z-levels) and publishes a
  JSON `MapSnapshot` to `<DF_DIR>/nivek-snapshot.json` via a temp file + atomic
  rename. It scans a time-boxed slice (~2 ms) per frame so DF never freezes;
  pass completion time scales with map depth (a deep fort is captured top to
  bottom, taking longer to finish than a shallow one). `df-snapshot-pusher`
  reads that file (`DASHBOARD_SNAPSHOT_FILE`) and forwards it. Wire shape
  matches `overseer.MapSnapshot` in `internal/libraries/overseer/const.go`.
