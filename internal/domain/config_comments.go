package domain

import (
	"errors"
	"strings"
)

// ConfigComment attaches one original Nginx comment to a directive in its form.
type ConfigComment struct {
	Anchor string `json:"anchor"`
	Text   string `json:"text"`
	Inline bool   `json:"inline,omitempty"`
	After  bool   `json:"after,omitempty"`
}

func validateConfigComments(comments []ConfigComment) error {
	for _, c := range comments {
		if !strings.HasPrefix(c.Text, "#") || strings.ContainsAny(c.Text, "\x00\r\n") {
			return errors.New("配置注释必须是以 # 开头的单行文本")
		}
	}
	return nil
}
