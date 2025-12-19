# Official Economic Calendar Sources

## Where to Get Official Data

### 1. Bureau of Labor Statistics (BLS)
**URL**: https://www.bls.gov/schedule/news_release/

**Releases:**
- **Employment Situation** (NFP + Unemployment): https://www.bls.gov/schedule/news_release/empsit.htm
- **Consumer Price Index (CPI)**: https://www.bls.gov/schedule/news_release/cpi.htm
- **Producer Price Index (PPI)**: https://www.bls.gov/schedule/news_release/ppi.htm

**Typical Schedule:**
- Employment Situation: First Friday of each month at 8:30 AM ET
- CPI: Mid-month (12th-15th) at 8:30 AM ET
- PPI: Mid-month (11th-14th) at 8:30 AM ET

### 2. U.S. Census Bureau
**URL**: https://www.census.gov/economic-indicators/

**Releases:**
- **Retail Sales**: https://www.census.gov/retail/marts/www/timeseries.html
- **Housing Starts**: https://www.census.gov/construction/nrc/

**Typical Schedule:**
- Retail Sales: Mid-month (13th-17th) at 8:30 AM ET
- Housing Starts: Mid-month at 8:30 AM ET

### 3. Federal Reserve
**URL**: https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm

**2025 FOMC Meeting Dates:**
```
Jan 28-29, 2025
Mar 18-19, 2025
May 6-7, 2025
Jun 17-18, 2025
Jul 29-30, 2025
Sep 16-17, 2025
Oct 28-29, 2025
Dec 9-10, 2025
```

Decision announced at 2:00 PM ET on the second day, followed by press conference at 2:30 PM ET.

### 4. Bureau of Economic Analysis (BEA)
**URL**: https://www.bea.gov/news/schedule

**Releases:**
- **GDP**: Quarterly (advance, preliminary, final estimates)

**Typical Schedule:**
- Q4 2024 Advance: Jan 30, 2025
- Q1 2025 Advance: Apr 30, 2025
- Q2 2025 Advance: Jul 30, 2025
- Q3 2025 Advance: Oct 30, 2025

---

## How to Get Exact Dates

### Method 1: BLS Economic News Release Schedule
Visit: https://www.bls.gov/schedule/2025/home.htm

Shows all BLS releases for the year with exact dates.

### Method 2: Economic Calendar Websites

**Investing.com**
https://www.investing.com/economic-calendar/
- Shows all major releases
- Free, no API needed
- Can export/scrape

**ForexFactory**
https://www.forexfactory.com/calendar
- Popular among traders
- Color-coded by impact
- Shows forecast/actual values

**TradingView**
https://www.tradingview.com/economic-calendar/
- Clean interface
- All major indicators
- Can filter by country/impact

### Method 3: Download Official Schedules

Each agency publishes annual schedules:

**BLS Annual Schedule (PDF/HTML)**
- https://www.bls.gov/schedule/2025/home.htm
- All releases for the year
- Can be parsed programmatically

**Fed FOMC Schedule (HTML)**
- https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm
- 8 meetings per year
- Published in advance

---

## Implementation Strategy

### Hybrid Approach Components:

**1. FRED API (Auto-updating)**
- JOLTS (Job Openings)
- Consumer Expenditure Surveys
- FOMC Press Releases (supplementary data)
- Various Fed data

**2. Hardcoded Major Indicators (Update quarterly)**
```go
// Calculate first Friday of each month for NFP
// Known dates for CPI, PPI (from BLS schedule)
// FOMC meetings from Fed calendar
// Retail Sales pattern
```

**3. Optional: Scrape/Import Once**
- Download BLS schedule HTML once
- Parse dates for the year
- Store in database
- Refresh yearly or quarterly

---

## Recommended Data Sources for Our App

### Primary (100% reliable):
1. **BLS Schedule**: https://www.bls.gov/schedule/2025/home.htm
   - NFP, CPI, PPI official dates
   - Published annually

2. **Fed Calendar**: https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm
   - FOMC meetings
   - Never changes once published

3. **FRED API**: What we tested
   - JOLTS and other supplementary data
   - Auto-updates daily

### Secondary (for manual verification):
- Investing.com calendar (comprehensive)
- ForexFactory (trader-focused)

---

## Next Steps

**Option A: Quick Implementation**
1. Hardcode 2025 dates from BLS schedule
2. Add FRED API for supplementary data
3. Update manually each quarter

**Option B: One-Time Scrape**
1. Parse BLS schedule HTML once
2. Store all 2025 dates
3. Set reminder to update Jan 2026

**Option C: Full Automation**
1. Scrape BLS schedule on startup
2. Parse HTML for dates
3. Auto-update yearly

Which approach do you prefer?
