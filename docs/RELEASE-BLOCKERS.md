# Release blockers

## Wizard-Blocks-LTC: mainnet MWEB block test

The Litecoin app must not ship until a real **mainnet** Litecoin block, taken
from the user's own synced Litecoin node, passes the byte-for-byte MWEB test
(`go test -tags release ./internal/bitcoin`). That test fails until the block
is in `internal/bitcoin/testdata/ltc/mainnet-<height>.hex`.

When the Litecoin node app is installed and synced, run this one line on the
Umbrel (it saves the current tip block as hex in the home folder):

```
C=fladnagmai-ltc-doge-node_litecoind_1; sudo docker exec $C sh -c 'litecoin-cli -datadir=/data -rpcport=9332 -rpcuser=$RPC_USER -rpcpassword=$RPC_PASS getblock "$(litecoin-cli -datadir=/data -rpcport=9332 -rpcuser=$RPC_USER -rpcpassword=$RPC_PASS getbestblockhash)" 0' > ~/ltc-mainnet-block.hex
```

Then send `~/ltc-mainnet-block.hex`. The container name is the Litecoin + Dogecoin Node
app's (fladnagmai-ltc-doge-node, service litecoind); if it changes, this
command is updated in the same change.

Until this passes, the LTC app is a test build: its version carries "test"
(0.2.0-test1), its store name and description say TEST, and its page shows a
TEST BUILD line.

Status: **open**.

## Rental readiness

Measured with `wbload` on a 4-core test box (the load tool and the engine
share the cores). These are the limits before the rental/public-port job.

### Wizard-Blocks-LTC: share-rate limit

- **1,000 connections:** about **2,000 shares/s** sustained (p99 0.5 s).
  At **4,000 shares/s offered**: 330 request timeouts, p99 12 s.
- **1 connection at rental rate** (50 TH/s): original target **2,910
  shares/s for 20 s with ≥ 90 % accepted: NOT MET on this 4-core box.**
  First run reached 967/s; a 3 s burst then offered 4,210 of the 7,857
  shares needed. The target was loosened twice after it failed (to "every
  offered share accepted" plus a reported 20 s hold). Reason: Scrypt costs
  about 1 ms per share on each side, and both sides share 4 cores. After
  the parallel share generator: burst 4,862 at 1,475/s, 20 s hold 30,901 at
  1,518/s, 0 rejects, 0 disconnects.
- Re-measure on the release hardware before calling LTC rental-ready.

### Connection limits

Defaults stay at **1,024 total and 64 per IP** (now adjustable in Settings,
admin password required). Raise them in the rental/public-port job, not
before.
