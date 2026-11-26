package nginx

import (
	"strings"
	"testing"

	"github.com/chenpingonline/nginx-web/internal/domain"
)

func TestRenderCommentsAtDirectiveBoundaries(t *testing.T) {
	const block = "server {\n    listen 9012;\n    location / {\n        return 200 \"# body\";\n    }\n}\n"
	comments := []domain.ConfigComment{
		{Text: "# header", Anchor: "/server:0"},
		{Text: "# listener", Anchor: "/server:0/listen:0", Inline: true},
		{Text: "# second note", Anchor: "/server:0/listen:0", Inline: true},
		{Text: "# nested", Anchor: "/server:0/location /:0/}"},
		{Text: "# footer", Anchor: "/server:0/}", After: true},
		{Text: "# removed directive", Anchor: "/server:0/rewrite:0"},
	}
	got := withConfigComments(block, comments)
	for _, want := range []string{"# header\n# removed directive\nserver {", "listen 9012; # listener\n    # second note\n", "return 200 \"# body\";\n    # nested\n    }", "}\n# footer\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}
