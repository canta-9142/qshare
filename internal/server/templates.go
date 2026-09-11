package server

import (
	"embed"
	"html/template"
)

//go:embed web/*.html
var webFiles embed.FS

var pageTemplates = template.Must(template.ParseFS(webFiles, "web/*.html"))
