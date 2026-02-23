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
		// Industrial and Materials
		"WM", "ETN", "SLB", "EOG", "MPC", "PSX", "VLO", "NSC",
		"FDX", "CSX", "ITW", "EMR", "HUM", "MCK", "COR", "CNC",
		"CI", "ELV", "BSX", "EW", "DXCM", "SYK", "ZBH", "STZ",
		"MNST", "KDP", "STZ", "GIS", "SYY", "ADM", "TSN", "K",
		"CHD", "CLX", "KMB", "EL", "PG", "TGT", "DG", "DLTR",
		"TJX", "ORLY", "AZO", "TSLA", "F", "GM", "MAR", "HLT",
		"RCL", "CCL", "NCLH", "LVS", "WYNN", "MGM", "BKNG", "EXPE",
		"CMG", "YUM", "DRI", "SBUX", "MCD", "NKE", "LULU", "HD",
		"LOW", "PH", "DHR", "TMO", "A", "MTD", "WAT", "PKI",
		"IQV", "D", "SO", "DUK", "AEP", "EXC", "XEL", "ED",
		"PEG", "SRE", "WEC", "ES", "DTE", "FE", "PPL", "AEE",
		"ETR", "CNP", "CMS", "LNT", "ATO", "PNW", "NI", "SR",
		"OGE", "EVRG", "PNW", "CEG", "VST", "NRG", "WRK", "AMCR",
		"IP", "VMC", "MLM", "SHW", "DD", "DOW", "APD", "LIN",
		"NEM", "FCX", "ALB", "FMC", "CTVA", "MOS", "CF", "NUE",
		"STLD", "RS", "T", "VZ", "TMUS", "LUMN", "CHTR", "CMCSA",
		"DIS", "NFLX", "PARA", "WBD", "FOXA", "FOX", "LYV", "EA",
		"TTWO", "MTCH", "IAC", "NYT", "IPG", "OMC", "GOOG", "GOOGL",
		"META", "AMZN", "EBAY", "ETSY", "BKNG", "EXPE", "ABNB", "BK",
		"STT", "NTRS", "BEN", "IVZ", "AMP", "TROW", "BLK", "MS",
		"GS", "JPM", "BAC", "WFC", "C", "PNC", "USB", "TFC",
		"COF", "DFS", "AXP", "V", "MA", "PYPL", "SQ", "AFRM",
		"HOOD", "COIN", "MSTR", "RIOT", "MARA", "CLSK", "BITO", "IBIT",
	}
}
