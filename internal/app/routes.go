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
	a.router.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static"))))
	a.router.HandleFunc("/eth-tx", ethTxHandler)
	a.router.HandleFunc("/about", gexTradingHandler)
	// Register the GEX handler
	tmpl := template.Must(template.ParseGlob("templates/*.html"))
	gexHandler := handler.NewGEXHandler(a.logger, tmpl, a.db)
	a.router.HandleFunc("/gex", gexHandler.ServeHTTP)
	a.router.HandleFunc("/", gexHandler.ServeHTTP)

}
