---
name: gex-report-tool
description: Generates a professional GEX (Gamma Exposure) regime report for a given stock symbol. Use when the user asks for a GEX summary, regime analysis, or NetGEX report for a ticker.
---

# GEX Report Tool

This skill generates a professional-grade Gamma Exposure (GEX) report using live Alpaca options data.

## Workflow

1.  **Generate Report**: Run the `generate_report.go` script with the desired symbol.
    ```bash
    go run scripts/generate_report.go <SYMBOL> [EXPIRY_DATE] [SPOT_PRICE]
    ```
    *   `<SYMBOL>`: The stock ticker (e.g., AAPL, ARM).
    *   `[EXPIRY_DATE]`: (Optional) The expiration date in YYYY-MM-DD format. Defaults to the nearest available expiry.
    *   `[SPOT_PRICE]`: (Optional) Manually override the spot price.

2.  **Formula**: The report uses the corrected Dollar Gamma formula:
    `GEX = Gamma * Open Interest * 100 * Spot Price`

## Output Format

The tool provides:
*   **Net GEX**: The combined gamma exposure (Calls - Puts).
*   **Total GEX**: The absolute total exposure.
*   **Gamma Condition**: Positive (Long Gamma) or Negative (Short Gamma).
*   **IV (Avg)**: Average implied volatility across the chain.
*   **Put/Call GEX Ratio**: Relative skew between put and call gamma.
*   **Distribution**: Top strikes by GEX above and below the spot price.

## Examples

*   "Give me a GEX report for ARM"
*   "Analyze the gamma regime for SPY for the next monthly expiry"
*   "What is the NetGEX for NVDA at a spot price of 900?"
