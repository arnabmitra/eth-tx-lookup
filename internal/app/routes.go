package app

import (
	"html/template"
	"net/http"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/handler"
	"github.com/arnabmitra/eth-proxy/internal/middleware"
)

func (a *App) loadRoutes(tmpl *template.Template) {
	guestbook := handler.New(a.logger, a.db, tmpl)
	ratelimiter := middleware.RateLimiter{
		Period:  time.Minute,
		MaxRate: 2,
		Store:   a.rdb,
	}

	files := http.FileServer(http.Dir("./static"))
	a.router.Handle("GET /static/", http.StripPrefix("/static", files))

	a.router.Handle("GET /{$}", http.HandlerFunc(guestbook.Home))

	a.router.Handle("POST /{$}", ratelimiter.Middleware(
		http.HandlerFunc(guestbook.Create),
	))
}
