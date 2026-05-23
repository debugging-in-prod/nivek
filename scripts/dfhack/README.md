# DFHack scripts (DFHost-side)

Lua scripts that run inside DFHack on the DFHost. They produce data the
rest of the nivek pipeline consumes — currently just the map snapshot
that feeds the `/df` dashboard on nivek.life.

## Installation on DFHost

DFHack auto-discovers scripts in `dfhack-config/scripts/` (relative to
the DF install directory). Symlink each script there so updates to this
repo flow through without copying:

```
ln -s "$PWD/scripts/dfhack/dump-map-snapshot.lua" \
  "/home/ne0n/.steam/debian-installation/steamapps/common/Dwarf Fortress/dfhack-config/scripts/dump-map-snapshot.lua"
```

After symlinking, the script is runnable by name:

```
dfhack-run dump-map-snapshot
```

The script writes JSON to stdout — pipe it where needed.

## Scripts

- **`dump-map-snapshot.lua`** — dumps the current fortress's tile + furniture
  data for the active Z level ± 2 (5 levels total) as a JSON `MapSnapshot`.
  Wire shape matches `overseer.MapSnapshot` in
  `internal/libraries/overseer/const.go`. Run via
  `dfhack-run dump-map-snapshot`.
