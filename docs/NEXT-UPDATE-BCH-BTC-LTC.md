# Next update: three lists

Nothing here is built or pinned yet. One new image after the 30/30 (run from
zero on the commit that gets built), then ask before building or pinning.

## LTC stuff (Wizard-Blocks² / Litecoin + Dogecoin)
- Done on phase4-fix: block check row showed a stray ",false" (996641b).
- Done on phase4-fix: stale shares are no longer counted as rejected (d1692d6).
- Fix the best-share units on the stats page: it shows raw share units while
  the network difficulty is in node units (65,536x smaller). Not approved yet.
- Drop the TEST label (mainnet MWEB byte-for-byte check passed 2026-10-13).
  User said leave it for now.
- 30/30 on the release commit, then the live testnet trial.

## BTC stuff
- Stale shares are no longer counted as rejected (same shared engine change,
  d1692d6). Reaches the BTC app only when it gets the new image.

## BCH stuff
- Stale shares are no longer counted as rejected (same shared engine change,
  d1692d6). Reaches the BCH app only when it gets the new image.
- Name change fix for the pixel-art party (internal/ui/static/js/app.js,
  `CHARACTERS` and `syncParty`). Today the wizard is always called "ernie" and
  each character is tied to one fixed miner name. Wanted:
  - Every character's name is the name of the miner it stands for.
  - If the first miner that connects is named "ernie", the fixed names work as
    today (ernie = wizard, chester = dwarf, ash = elf, bandit = dog, kaido =
    angel, molly = Molly).
  - If the first miner is not named "ernie", ignore all the fixed names: the
    characters go out in order, one per miner in the order the miners
    connected (wizard, dwarf, elf, dog, angel, Molly, the order in the current
    party code), each named after its miner. Extra miners get the generated
    characters as today.
