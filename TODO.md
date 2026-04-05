- execution
- backtesting engine
- persistence
- observability/logging
- dashboard

## weekend tasks
1. setup persistence layer (postgres) and docker compose
2. seed database with historical bars from 2025
3. paper broker for backtest execution
4. basic dashboard showing
    - all symbols with live price updates via websocket
    - canidates
    - pause/resume button


# important tasks before monday
- canidate ranking
    1. atr / price (relative volatility), volume (top volume gainers)
    2. using alpaca top stocks api
    3. should also filter duplicates and use priority queue for sorting by priority (timestamp, rank)
- execution engine
    1. mutex
    2. memory only persistence with subscription to updates from alpaca
