package worker

// SP500Symbols returns the list of S&P 500 stock symbols
// This is a subset for now - can be expanded to full 500
func SP500Symbols() []string {
	return []string{
		// Top weighted stocks in S&P 500
		"AAPL", "MSFT", "NVDA", "AMZN", "GOOGL", "META", "TSLA", "BRK.B",
		"AVGO", "LLY", "JPM", "UNH", "XOM", "V", "MA", "PG",
		"COST", "JNJ", "HD", "ABBV", "NFLX", "CRM", "BAC", "CVX",
		"KO", "WMT", "MRK", "ORCL", "AMD", "PEP", "TMO", "ADBE",
		"ACN", "LIN", "CSCO", "MCD", "ABT", "DHR", "INTC", "TXN",
		"NKE", "DIS", "VZ", "CMCSA", "WFC", "PM", "COP", "NEE",
		"IBM", "QCOM", "UNP", "RTX", "INTU", "LOW", "AMGN", "HON",
		"GE", "BA", "SPGI", "CAT", "BLK", "UPS", "SBUX", "AXP",
		"DE", "GILD", "ELV", "BKNG", "ADI", "PLD", "MMC", "TJX",
		"MDLZ", "VRTX", "SYK", "ADP", "ISRG", "CI", "REGN", "AMT",
		"ZTS", "PGR", "SCHW", "CB", "SO", "DUK", "CME", "BSX",
		"ETN", "FISV", "MO", "ITW", "BDX", "APH", "MMM", "NOC",
		"HCA", "PNC", "GD", "CL", "USB", "SHW", "AON", "EMR",
		// Additional major stocks
		"MU", "PANW", "SNPS", "CDNS", "KLAC", "AMAT", "LRCX", "MRVL",
		"FTNT", "CRWD", "DDOG", "NET", "ZS", "SNOW", "TEAM", "NOW",
		"WDAY", "PLTR", "SQ", "SHOP", "ROKU", "ZM", "DOCU", "OKTA",
	}
}
