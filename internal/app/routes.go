package app

import (
	"github.com/arnabmitra/eth-proxy/internal/handler"
	"html/template"
	"net/http"
)

func (a *App) loadRoutes() {
	a.router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	a.router.HandleFunc("/btc-etf", btcEtfHandler)
	// Serve static files from the "static" directory
	files := http.FileServer(http.Dir("./static"))
	a.router.Handle("/static/", http.StripPrefix("/static", files))

	// Register the handler function before starting the server
	a.router.HandleFunc("/eth-tx", ethTxHandler)
	a.router.HandleFunc("/about", gexTradingHandler)
	// Register the GEX handler
	tmpl := template.Must(template.ParseGlob("templates/*.html"))
	a.router.HandleFunc("/gex", handler.NewGEXHandler(a.logger, tmpl).ServeHTTP)

	a.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/gex", http.StatusSeeOther)
	})

}
