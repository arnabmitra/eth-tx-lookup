package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/arnabmitra/eth-proxy/internal/blog"
)

type BlogHandler struct {
	logger *slog.Logger
	tmpl   *template.Template
	system *blog.BlogSystem
}

func NewBlogHandler(logger *slog.Logger, tmpl *template.Template) *BlogHandler {
	system := blog.NewBlogSystem()
	err := system.LoadPosts("posts") // Assuming posts directory is in root
	if err != nil {
		logger.Error("failed to load blog posts", "error", err)
	}

	return &BlogHandler{
		logger: logger,
		tmpl:   tmpl,
		system: system,
	}
}

func (h *BlogHandler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	posts := h.system.GetAllPosts()
	err := h.tmpl.ExecuteTemplate(w, "blog_index.html", map[string]interface{}{
		"Posts": posts,
	})
	if err != nil {
		h.logger.Error("failed to render blog index", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *BlogHandler) ServePost(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/blog/")
	if slug == "" {
		h.ServeIndex(w, r)
		return
	}
	
	post, found := h.system.GetPost(slug)
	if !found {
		http.NotFound(w, r)
		return
	}

	err := h.tmpl.ExecuteTemplate(w, "blog_post.html", map[string]interface{}{
		"Post": post,
	})
	if err != nil {
		h.logger.Error("failed to render blog post", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
