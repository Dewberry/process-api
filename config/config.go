package config

import (
	"io"
	"text/template"

	"github.com/labstack/echo/v4"
)

// Store for templates and a receiver function to render them
type Template struct {
	templates *template.Template
}

// Render the named template with the data
func (t Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// Store configuration for the API
type APIConfig struct {
	T Template
}

// Init initializes the API's configuration
func Init() APIConfig {
	config := APIConfig{}
	config.T = Template{
		templates: template.Must(template.ParseGlob("public/views/*.html")),
	}
	return config
}
