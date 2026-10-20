# Read by apps that depend on this one (Wizard-Blocks-LTC) and by this app's
# own docker-compose.yml. derive_entropy is umbrelOS's helper for stable
# per-install secrets (from the device seed).
export APP_LITECOIN_NODE_IP="10.21.21.140"
export APP_LITECOIN_RPC_PORT="9332"
export APP_LITECOIN_P2P_PORT="9333"
export APP_LITECOIN_ZMQ_HASHBLOCK_PORT="28332"
export APP_LITECOIN_RPC_USER="umbrel"
export APP_LITECOIN_RPC_PASS="$(derive_entropy "fladnagmai-ltc-doge-node-litecoin-rpc-password")"
export APP_LITECOIN_NETWORK="mainnet"

export APP_DOGECOIN_NODE_IP="10.21.21.141"
export APP_DOGECOIN_RPC_PORT="22555"
export APP_DOGECOIN_P2P_PORT="22556"
export APP_DOGECOIN_ZMQ_HASHBLOCK_PORT="28332"
export APP_DOGECOIN_RPC_USER="umbrel"
export APP_DOGECOIN_RPC_PASS="$(derive_entropy "fladnagmai-ltc-doge-node-dogecoin-rpc-password")"
export APP_DOGECOIN_NETWORK="mainnet"

export APP_PEPECOIN_NODE_IP="10.21.21.142"
export APP_PEPECOIN_RPC_PORT="33873"
export APP_PEPECOIN_P2P_PORT="33874"
export APP_PEPECOIN_ZMQ_HASHBLOCK_PORT="28332"
export APP_PEPECOIN_RPC_USER="umbrel"
export APP_PEPECOIN_RPC_PASS="$(derive_entropy "fladnagmai-ltc-doge-node-pepecoin-rpc-password")"
export APP_PEPECOIN_NETWORK="mainnet"
# Full URLs, so a dependent app can leave Pepecoin off (empty) while this
# app is still an older version without a Pepecoin node.
export APP_PEPECOIN_RPC_URL="http://${APP_PEPECOIN_NODE_IP}:${APP_PEPECOIN_RPC_PORT}"
export APP_PEPECOIN_ZMQ_HASHBLOCK_URL="tcp://${APP_PEPECOIN_NODE_IP}:${APP_PEPECOIN_ZMQ_HASHBLOCK_PORT}"
