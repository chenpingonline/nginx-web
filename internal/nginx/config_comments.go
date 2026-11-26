package nginx

import (
	"io"
	"strconv"
	"strings"

	"github.com/chenpingonline/nginx-web/internal/domain"
	crossplane "github.com/nginxinc/nginx-go-crossplane"
)

// Resolve anchors against the current render, so edits to a directive's values
// do not detach its comments. Keep orphaned comments above their owning block.
func withConfigComments(block string, comments []domain.ConfigComment) string {
	if len(comments) == 0 {
		return block
	}
	lines := strings.Split(block, "\n")
	anchors := map[string]int{}
	parsed, err := crossplane.Parse("block.conf", &crossplane.ParseOptions{
		SingleFile: true, SkipDirectiveContextCheck: true, SkipDirectiveArgsCheck: true,
		Open: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(block)), nil },
	})
	if err == nil && len(parsed.Config) == 1 && len(parsed.Errors) == 0 {
		multiline := false
		var index func(crossplane.Directives, string, int)
		index = func(nodes crossplane.Directives, parent string, limit int) {
			counts := map[string]int{}
			for i, node := range nodes {
				for _, arg := range node.Args {
					if strings.ContainsAny(arg, "\r\n") {
						multiline = true
					}
				}
				key := node.Directive
				if key == "location" {
					key += " " + strings.Join(node.Args, " ")
				}
				path := parent + "/" + key + ":" + strconv.Itoa(counts[key])
				counts[key]++
				anchors[path] = node.Line - 1
				if node.IsBlock() {
					end := limit - 1
					if i+1 < len(nodes) {
						end = nodes[i+1].Line - 2
					}
					for end > node.Line-1 && strings.TrimSpace(lines[end]) == "" {
						end--
					}
					anchors[path+"/}"] = end
					index(node.Block, path, end)
				}
			}
		}
		index(parsed.Config[0].Parsed, "", len(lines))
		// A physical line inside a quoted argument is not a safe insertion point.
		if multiline {
			clear(anchors)
		}
	}
	before, after := map[int][]string{}, map[int][]string{}
	inlineUsed := map[int]bool{}
	for _, c := range comments {
		// Also protect previews rendered before state validation.
		if !strings.HasPrefix(c.Text, "#") || strings.ContainsAny(c.Text, "\r\n\x00") {
			continue
		}
		line, ok := anchors[c.Anchor]
		if !ok || line < 0 || line >= len(lines) {
			before[0] = append(before[0], c.Text)
			continue
		}
		indent := lines[line][:len(lines[line])-len(strings.TrimLeft(lines[line], " \t"))]
		if c.Inline && !inlineUsed[line] {
			lines[line] += " " + c.Text
			inlineUsed[line] = true
		} else if c.After || c.Inline {
			after[line] = append(after[line], indent+c.Text)
		} else {
			before[line] = append(before[line], indent+c.Text)
		}
	}
	var result []string
	for i, line := range lines {
		result = append(result, before[i]...)
		result = append(result, line)
		result = append(result, after[i]...)
	}
	return strings.Join(result, "\n")
}
