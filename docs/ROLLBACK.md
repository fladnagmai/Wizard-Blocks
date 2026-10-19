# Rolling back the unified-UI update

Use this to go back to the versions that were live before the unify-ui update.
Every command is one line. Run them where it says.

## What you go back to

| App | App id | Version | Image |
|---|---|---|---|
| Wizard-Blocks-BCH | `fladnagmai-wizard-blocks` | 0.1.8-alpha | `ghcr.io/fladnagmai/wizard-blocks:0.1.8-alpha@sha256:20f7af12e736bbf092fd4e954ae758602a2ca3745372aefe9547c44ea10749ba` |
| Wizard-Blocks-BTC | `fladnagmai-wizard-blocks-btc` | 0.1.8-alpha | same image as BCH |
| Wizard-Blocks² (test) | `fladnagmai-wizard-blocks-ltc` | 0.2.0-test1 | `ghcr.io/fladnagmai/wizard-blocks:0.2.0-test1@sha256:68426355fe57e34fd6b902704bad3c4e43ad6b0e5d779411b2a542fd87db66b3` |

The store files for these versions are on the branch `backup/pre-unify-2026-10-19`
(commit `3b3d6d965d386962f98488efe2fc22e840662207`, the old `main`). Nothing was
deleted or rewritten: the old images stay in the registry under those digests.

The saved settings, payout addresses, passwords and history are in each app's
data folder. The update does not change any file format, so the old version
reads the same files. The data backup is a second safety net.

## 1. Before the update: back up each app's data folder (on the Umbrel, over ssh)

```
sudo cp -a ~/umbrel/app-data/fladnagmai-wizard-blocks ~/wb-backup-bch-2026-10-19
```
```
sudo cp -a ~/umbrel/app-data/fladnagmai-wizard-blocks-btc ~/wb-backup-btc-2026-10-19
```
```
sudo cp -a ~/umbrel/app-data/fladnagmai-wizard-blocks-ltc ~/wb-backup-ltc-2026-10-19
```

## 2. Revert the store on GitHub (on your computer)

```
git clone https://github.com/fladnagmai/Wizard-Blocks.git wb-rollback
```
```
cd wb-rollback
```
```
git fetch origin backup/pre-unify-2026-10-19
```
```
git checkout origin/backup/pre-unify-2026-10-19 -- fladnagmai-wizard-blocks fladnagmai-wizard-blocks-btc fladnagmai-wizard-blocks-ltc umbrel-app-store.yml
```
```
git commit -m "Roll back the three apps to the versions before the unified UI"
```
```
git push origin main
```

This puts the three app folders back exactly as they were, pins included.

## 3. Reinstall the old version on the Umbrel

Umbrel does not install an older version over a newer one, so each app is
uninstalled and installed again. In the Umbrel web page: App Store, open
Community Stores, refresh the store, then for each app: open it, Uninstall,
then Install. Uninstalling removes its data folder; the next step puts it back.

## 4. Put the saved data back (on the Umbrel, over ssh)

Do this for each app once the reinstalled app has started at least once.

BCH:
```
sudo docker stop fladnagmai-wizard-blocks_engine_1
```
```
sudo cp -a ~/wb-backup-bch-2026-10-19/data/. ~/umbrel/app-data/fladnagmai-wizard-blocks/data/
```
```
sudo docker start fladnagmai-wizard-blocks_engine_1
```

BTC:
```
sudo docker stop fladnagmai-wizard-blocks-btc_engine_1
```
```
sudo cp -a ~/wb-backup-btc-2026-10-19/data/. ~/umbrel/app-data/fladnagmai-wizard-blocks-btc/data/
```
```
sudo docker start fladnagmai-wizard-blocks-btc_engine_1
```

LTC + DOGE:
```
sudo docker stop fladnagmai-wizard-blocks-ltc_engine_1
```
```
sudo cp -a ~/wb-backup-ltc-2026-10-19/data/. ~/umbrel/app-data/fladnagmai-wizard-blocks-ltc/data/
```
```
sudo docker start fladnagmai-wizard-blocks-ltc_engine_1
```

## 5. Check

Each line must show the image from the table above:

```
sudo docker inspect --format '{{.Config.Image}}' fladnagmai-wizard-blocks_engine_1
```
```
sudo docker inspect --format '{{.Config.Image}}' fladnagmai-wizard-blocks-btc_engine_1
```
```
sudo docker inspect --format '{{.Config.Image}}' fladnagmai-wizard-blocks-ltc_engine_1
```

Then open each app: the payout address, difficulty settings and (LTC) the
connection limits are the ones you had.

## Tested

`test/integration/upgrade_test.go` starts the old engine on a data folder with
saved settings, starts the new engine on the same folder, then the old engine
again on that folder. See the unify-ui report for the results.
