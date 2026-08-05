package console

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed ui
var embeddedUI embed.FS

func uiFileSystem() (fs.FS, error) {
	result, err := fs.Sub(embeddedUI, "ui")
	if err != nil {
		return nil, fmt.Errorf("open embedded console UI: %w", err)
	}
	return result, nil
}
