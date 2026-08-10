package web

import (
	"embed"
)

// TemplateFS embeds all HTML templates.
//go:embed templates/*.html templates/partials/*.html
var TemplateFS embed.FS

// StaticFS embeds static asset files if present.
//go:embed static
var StaticFS embed.FS
