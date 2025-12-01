# Navigation Template Usage

## Overview
The navigation template is defined in `templates/navigation.html` and can be included in any page.

## ⚠️ IMPORTANT: Two-Step Process Required

You must do BOTH steps together for each page:

### Step 1: Update app.go handler
Change the handler to load both templates:
```go
// Before:
tmpl, err := template.ParseFiles("templates/yourpage.html")

// After:
tmpl, err := template.ParseFiles("templates/navigation.html", "templates/yourpage.html")
```

### Step 2: Update the HTML file
Replace the existing `<header>` section with:
```html
{{template "navigation"}}
```

⚠️ **Do NOT update app.go without updating the HTML file, or pages won't render!**

### Example - Before and After

**Before (old way):**
```html
<body>
    <header class="bg-gray-900/80...">
        <div class="container...">
            <!-- 200+ lines of navigation code -->
        </div>
    </header>
    
    <main>
        <!-- Your content -->
    </main>
</body>
```

**After (new way):**
```html
<body>
    {{template "navigation"}}
    
    <main>
        <!-- Your content -->
    </main>
</body>
```

## Navigation Structure

### Top-Level Menu Items:
1. Tracker (`/gex`)
2. All Expiries (`/all-gex`)
3. MAG7 (`/mag7-gex`)
4. Strategies (`/strategies`)
5. FAQ (`/faq`)

### More Dropdown:
1. GEX History (`/gex-history?symbol=SPY&limit=5`)
2. BTC ETF (`/btc-etf`)
3. Learn Center (`/about`)
4. About Us (`/about-us`)
5. Glossary (`/glossary`)

## Benefits
- ✅ Single source of truth for navigation
- ✅ Easy to update - change once, applies everywhere
- ✅ Reduces code duplication
- ✅ Maintains consistency across all pages
- ✅ Easier to add/remove menu items

## Updating Navigation
To add/remove/change menu items, edit only:
- `templates/navigation.html`

All pages using `{{template "navigation"}}` will automatically get the updates!
