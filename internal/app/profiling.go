package app

import (
	"net/http/pprof"
)

func (a *App) loadProfilingRoutes() {
	a.router.HandleFunc("/debug/pprof/", pprof.Index)
	a.router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	a.router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	a.router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	a.router.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
