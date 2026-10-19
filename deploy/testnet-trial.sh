#!/usr/bin/env bash
# Live testnet trial of the BCH or BTC engine: its own testnet node plus the
# published release image, separate from the mainnet Umbrel apps (own
# containers, volumes and LAN ports; nothing is opened on the router).
#
#   sudo bash deploy/testnet-trial.sh btc start    # Bitcoin testnet4
#   sudo bash deploy/testnet-trial.sh bch start    # Bitcoin Cash chipnet
#   sudo bash deploy/testnet-trial.sh btc status   # sync, shares, blocks
#   sudo bash deploy/testnet-trial.sh btc stop     # stop (keeps the chain)
#   sudo bash deploy/testnet-trial.sh btc remove   # stop and delete everything
set -euo pipefail
coin=${1:-}; cmd=${2:-start}
case $coin in
  btc) svc=bitcoind; net=-testnet4; ui=8422; explorer="https://mempool.space/testnet4/address/" ;;
  bch) svc=bchn; net=-chipnet; ui=8421; explorer="https://chipnet.imaginary.cash/address/" ;;
  *) echo "usage: $0 btc|bch start|status|stop|remove" >&2; exit 2 ;;
esac
cd "$(dirname "$0")/$coin-testnet"
ip=$(hostname -I | awk '{print $1}')

cli() {
  # shellcheck disable=SC1091
  . ./.env
  docker compose exec -T "$svc" bitcoin-cli "$net" -rpcport=18332 -rpcuser="$RPC_USER" -rpcpassword="$RPC_PASSWORD" "$@"
}

case $cmd in
start)
  if [ ! -f .env ]; then
    cp .env.example .env
    sed -i "s/^RPC_PASSWORD=.*/RPC_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')/" .env
    # Keep the testnet chain small on the Umbrel's disk.
    sed -i 's/^BITCOIND_PRUNE_MB=.*/BITCOIND_PRUNE_MB=2000/; s/^BCHN_PRUNE_MB=.*/BCHN_PRUNE_MB=2000/' .env
  fi
  docker compose pull -q
  docker compose up -d "$svc"
  echo "waiting for the $coin testnet node..."
  for _ in $(seq 120); do cli getblockchaininfo >/dev/null 2>&1 && break; sleep 2; done
  cli getblockchaininfo >/dev/null
  cli createwallet payout >/dev/null 2>&1 || cli loadwallet payout >/dev/null 2>&1 || true
  if ! grep -q '^PAYOUT_ADDRESS=.' .env; then
    addr=$(cli -rpcwallet=payout getnewaddress | tr -d '\r')
    sed -i "s|^PAYOUT_ADDRESS=.*|PAYOUT_ADDRESS=$addr|" .env
  fi
  docker compose up -d engine
  . ./.env
  echo
  echo "Started. The engine waits until the node is synced before it gives out work."
  echo "  Miner pool URL : stratum+tcp://$ip:$STRATUM_PORT"
  echo "  Miner user     : any name    password: x"
  echo "  Web page       : http://$ip:$ui"
  echo "  Payout address : $PAYOUT_ADDRESS"
  echo "  Explorer       : $explorer$PAYOUT_ADDRESS"
  echo "Check progress with: sudo bash deploy/testnet-trial.sh $coin status"
  ;;
status)
  . ./.env
  cli getblockchaininfo | grep -E '"(chain|blocks|headers|initialblockdownload|verificationprogress)"'
  echo "--- engine (last block and share lines) ---"
  docker compose logs --no-color engine 2>&1 | grep -E "BLOCK|block check|miner authorized|waiting for node|synced|refusing" | tail -15
  echo "Payout address : $PAYOUT_ADDRESS"
  echo "Explorer       : $explorer$PAYOUT_ADDRESS"
  echo "Web page       : http://$ip:$ui"
  ;;
stop) docker compose down ;;
remove) docker compose down -v; rm -f .env ;;
*) echo "usage: $0 btc|bch start|status|stop|remove" >&2; exit 2 ;;
esac
