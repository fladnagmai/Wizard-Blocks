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
