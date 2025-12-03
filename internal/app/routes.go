package app

import (
	"github.com/arnabmitra/eth-proxy/internal/handler"
	"html/template"
	"net/http"
)

func (a *App) loadRoutes() *handler.GEXHandler {
	a.router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	a.router.HandleFunc("/btc-etf", btcEtfHandler)
	a.router.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static"))))
	
	// Serve SEO and verification files from root
	a.router.HandleFunc("/ads.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.ServeFile(w, r, "./static/ads.txt")
	})
	
	a.router.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.ServeFile(w, r, "./static/robots.txt")
	})
	
	a.router.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		http.ServeFile(w, r, "./static/sitemap.xml")
	})
	
	a.router.HandleFunc("/eth-tx", ethTxHandler)
	a.router.HandleFunc("/about", gexTradingHandler)
	a.router.HandleFunc("/strategies", strategiesHandler)
	a.router.HandleFunc("/faq", faqHandler)
	a.router.HandleFunc("/glossary", glossaryHandler)
	a.router.HandleFunc("/privacy", privacyHandler)
	a.router.HandleFunc("/terms", termsHandler)
	a.router.HandleFunc("/cookies", cookiesHandler)
	a.router.HandleFunc("/about-us", aboutUsHandler)
	// Register the GEX handler
	tmpl := template.Must(template.ParseGlob("templates/*.html"))
	gexHandler := handler.NewGEXHandler(a.logger, tmpl, a.db)
	a.router.HandleFunc("/gex", gexHandler.ServeHTTP)
	a.router.HandleFunc("/", gexTradingHandler)
	// Register the expiry dates handler
	a.router.HandleFunc("/expiry-dates", gexHandler.GetExpiryDatesHandler)
	// Add this new route for the all-expiry GEX page
	a.router.HandleFunc("/all-gex", gexHandler.AllGEXHandler)

	a.router.HandleFunc("/gex-history", gexHandler.DisplayGEXHistoryPage)
	a.router.HandleFunc("/mag7-gex", gexHandler.MAG7GEXHandler)
	return gexHandler
}
