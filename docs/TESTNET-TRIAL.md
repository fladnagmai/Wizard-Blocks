# Testnet trial checklist (Wizard-Blocks-LTC)

Run on the Umbrel with the Litecoin + Dogecoin Node app on **testnet**
(Litecoin testnet4, Dogecoin testnet) and the Elphapex DG Home 1 pointed at
the LTC app's Stratum port. Tick each item and note the time and what was
seen. Anything that does not behave as written is a stop: report it with the
engine log (`sudo docker logs fladnagmai-wizard-blocks-ltc_engine_1`).

## Setup
- [ ] Both nodes synced on the node app page (status "synced", heights match
      a testnet explorer).
- [ ] LTC payout (testnet address) and DOGE payout (testnet address) saved
      on the LTC page; a mainnet address is refused with the reason.
- [ ] DG Home 1 connects; first difficulty 262144 (or its d= value); share
      rate settles near one share per 10 s.

## Mining
- [ ] Shares accepted; rejects only "stale" right after new blocks.
- [ ] DOGE blocks found are accepted by the Dogecoin node and appear on a
      Dogecoin testnet explorer with our payout address.
- [ ] Any LTC block found is accepted and appears on a Litecoin testnet4
      explorer (MWEB block format).
- [ ] A new Dogecoin block gives the miner a fresh job within seconds.

## Dogecoin sync paused (cannot be seen on regtest)
Regtest skips dogecoind's peer check, so this is only visible on a real
network. Unit tests cover it (`TestDogeMergedMiningOffWhileNoPeers`,
`TestAuxWithdrawnDropsTag`); this confirms it end to end.
- [ ] On the node app page press **pause Dogecoin sync**: the Dogecoin node
      shows "PAUSED (sync stopped)" and 0 peers.
- [ ] Within one poll the LTC page's DOGE NODE panel shows
      "merged mining: no (node has no peers)" and the CHAINS row for DOGE
      shows merged "no".
- [ ] The miner gets a clean job; the engine log shows "no aux work: mining
      the parent chain alone"; LTC shares are still accepted.
- [ ] Restart the Dogecoin container (`sudo docker restart
      fladnagmai-ltc-doge-node_dogecoind_1`): it comes back still paused.
- [ ] Press **resume Dogecoin sync**: peers return, merged mining is back on
      ("merged mining: yes") within a minute, the miner gets a clean job
      with the merged-mining tag, and DOGE shares count again.

## Pepecoin merged mining (third chain)
Pepecoin's testnet uses the same address version bytes as Dogecoin's
testnet ("n…"), so check which node made each address.
- [ ] The node app page shows the Pepecoin node synced on testnet, its
      memory use, and the memory total of all three nodes.
- [ ] Before a PEP address is set, the LTC page shows "Pepecoin is not
      merged: add a PEP payout address"; LTC and DOGE are mined.
- [ ] Save a PEP testnet address (from the Pepecoin node:
      `sudo docker exec fladnagmai-ltc-doge-node_pepecoind_1 sh -c 'pepecoin-cli -testnet -rpcport=44873 -rpcuser=$RPC_USER -rpcpassword=$RPC_PASS getnewaddress'`);
      the PEP panel shows "merged" and the title reads LTC + DOGE + PEP.
- [ ] PEP blocks found are accepted by the Pepecoin node and pay the PEP
      address.
- [ ] Pause the Pepecoin sync on the node app page: the LTC page says
      Pepecoin is not merged (no peers), LTC and DOGE carry on; resume: PEP
      is merged again within a minute.
- [ ] Stop the Pepecoin container
      (`sudo docker stop fladnagmai-ltc-doge-node_pepecoind_1`): "node down",
      LTC and DOGE carry on; start it again: PEP merged again.

## Pruning
- [ ] Change the Litecoin prune target on the node app page: only the
      Litecoin node restarts, the LTC page shows the node reconnecting, then
      mining continues without manual action.
- [ ] Same for Dogecoin and Pepecoin (that chain's merged mining is off while
      it restarts, back after).

## Restarts
- [ ] Restart the LTC app: payouts, blocks and settings are kept.
- [ ] Reboot the Umbrel: both apps come back and mining resumes on its own.

## Then
Mainnet only after the mainnet MWEB block test passes
(docs/RELEASE-BLOCKERS.md) and this list is complete.
