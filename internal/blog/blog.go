package blog

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type Post struct {
	Title         string
	Slug          string
	Date          time.Time
	Summary       string
	Image         string
	ContentHTML   template.HTML
	FormattedDate string
}

type BlogSystem struct {
	posts    []Post
	postsMap map[string]Post
}

func NewBlogSystem() *BlogSystem {
	return &BlogSystem{
		posts:    make([]Post, 0),
		postsMap: make(map[string]Post),
	}
}

func (b *BlogSystem) LoadPosts(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return err
	}

	markdown := goldmark.New(
		goldmark.WithExtensions(meta.Meta),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(), // Allow raw HTML in markdown
		),
	)

	var loadedPosts []Post
	postsMap := make(map[string]Post)

	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Failed to read file %s: %v\n", file, err)
			continue
		}

		var buf bytes.Buffer
		context := parser.NewContext()
		if err := markdown.Convert(source, &buf, parser.WithContext(context)); err != nil {
			fmt.Printf("Failed to parse markdown %s: %v\n", file, err)
			continue
		}

		metaData := meta.Get(context)
		
		title, _ := metaData["title"].(string)
		slug, _ := metaData["slug"].(string)
		dateStr, _ := metaData["date"].(string)
		summary, _ := metaData["summary"].(string)
		image, _ := metaData["image"].(string)

		if slug == "" {
			// Fallback: use filename as slug
			filename := filepath.Base(file)
			slug = filename[:len(filename)-len(filepath.Ext(filename))]
		}

		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			// Try other format or default to now
			parsedDate = time.Now()
		}

		post := Post{
			Title:         title,
			Slug:          slug,
			Date:          parsedDate,
			Summary:       summary,
			Image:         image,
			ContentHTML:   template.HTML(buf.String()),
			FormattedDate: parsedDate.Format("January 02, 2006"),
		}

		loadedPosts = append(loadedPosts, post)
		postsMap[slug] = post
	}

	// Sort by date descending
	sort.Slice(loadedPosts, func(i, j int) bool {
		return loadedPosts[i].Date.After(loadedPosts[j].Date)
	})

	b.posts = loadedPosts
	b.postsMap = postsMap
	return nil
}

func (b *BlogSystem) GetAllPosts() []Post {
	return b.posts
}

func (b *BlogSystem) GetPost(slug string) (Post, bool) {
	post, ok := b.postsMap[slug]
	return post, ok
}
