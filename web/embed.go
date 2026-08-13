package web

import (
	"embed"
)

// TemplateFS embeds all HTML templates.
//go:embed templates/*.html templates/partials/*.html
var TemplateFS embed.FS
