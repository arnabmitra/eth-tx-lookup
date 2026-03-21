from alpaca.trading.client import TradingClient
from alpaca.data.historical import OptionHistoricalDataClient
from alpaca.data.requests import OptionChainRequest, OptionSnapshotRequest
from alpaca.trading.requests import GetOptionContractsRequest
from datetime import datetime

# Credentials
API_KEY = "PK5QDVG6M44WI2LBZNXKPB4Y4E"
SECRET_KEY = "2srHM4WVZSZVEhcBweAVhsvzoRtKnUPMtQYhUEMFJkzq"

# Initialize Clients
trading_client = TradingClient(API_KEY, SECRET_KEY, paper=True)
data_client = OptionHistoricalDataClient(API_KEY, SECRET_KEY)

symbol = "SPY"

print(f"--- Fetching data for {symbol} ---")

# 1. Get Expiration Dates
print("\n--- Expiration Dates ---")
request_params = GetOptionContractsRequest(
    underlying_symbols=[symbol],
    status="active"
)
contracts = trading_client.get_option_contracts(request_params)

expirations = sorted(list(set([c.expiration_date for c in contracts])))
print(f"Found {len(expirations)} expiration dates. First: {expirations[0]}")

first_exp = expirations[0]

# 2. Get Option Chain for first expiration
print(f"\n--- Option Chain for {symbol} (Exp: {first_exp}) ---")

# We need to filter contracts for this expiration to show Open Interest
exp_contracts = [c for c in contracts if c.expiration_date == first_exp]
print(f"Found {len(exp_contracts)} contracts for this expiration.")

# Get Snapshots for Greeks
# Note: get_option_chain for snapshots is a beta feature or use OptionSnapshotRequest
snapshot_params = OptionSnapshotRequest(underlying_symbol=symbol)
snapshots = data_client.get_option_snapshot(snapshot_params)

# Combine and show a few
count = 0
for contract in exp_contracts[:5]:
    snap = snapshots.get(contract.symbol)
    gamma = snap.greeks.gamma if snap and snap.greeks else 0
    print(f"Contract: {contract.symbol}")
    print(f"  Strike: {contract.strike_price} | Type: {contract.type} | OI: {contract.open_interest}")
    print(f"  Bid: {snap.latest_quote.bid_price if snap else 'N/A'} | Gamma: {gamma}")
    count += 1
