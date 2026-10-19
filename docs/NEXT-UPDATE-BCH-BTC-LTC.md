# Next update: three lists

Nothing here is built or pinned yet. One new image after the 30/30 (run from
zero on the commit that gets built), then ask before building or pinning.

## All apps (done on phase4-fix, in the next image)
- Chance of hitting a block is shown as odds "1:N" instead of a percentage:
  BCH page (block / day, week, year), BTC page (new row: block odds day /
  week), LTC page (new pool row and a per-chain column, so Dogecoin too).

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
- NOTE: on the unify-ui branch the BCH app uses the BTC page, so the
  pixel-art party below is not shown by any app. The rule stays here in case
  the game page comes back.
- Name change fix for the pixel-art party (internal/ui/static/js/app.js,
  `CHARACTERS` and `syncParty`). Today the wizard is always called "ernie" and
  each character is tied to one fixed miner name. Wanted:
  - The characters have no names of their own.
  - If the first miner that connects is named "ernie", the characters take
    the names of the specific miners as today (ernie = wizard, chester =
    dwarf, ash = elf, bandit = dog, kaido = angel, molly = Molly).
  - Otherwise ignore all of those fixed names. Each character that shows up is
    named after the actual miner whose connection made it show up, in the
    order the miners connect.
