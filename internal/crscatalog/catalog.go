// Package crscatalog lists inbound detection rules from the same embedded CRS
// files that the inspector loads. Administrative and scoring rules stay internal.
package crscatalog

import (
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	crs "github.com/corazawaf/coraza-coreruleset/v4"
)

type Rule struct {
	ID       int    `json:"id"`
	Message  string `json:"message"`
	Category string `json:"category"`
	Level    int    `json:"level"`
}

var catalog = sync.OnceValues(func() ([]Rule, map[int]bool) {
	id := regexp.MustCompile(`\bid:(\d+)`)
	message := regexp.MustCompile(`\bmsg:'((?:\\.|[^'\\])*)'`)
	level := regexp.MustCompile(`tag:'paranoia-level/([1-4])'`)
	files, err := fs.Glob(crs.FS, "@owasp_crs/REQUEST-*.conf")
	if err != nil {
		panic(err)
	}
	rules := []Rule{}
	known := map[int]bool{}
	for _, file := range files {
		content, err := fs.ReadFile(crs.FS, file)
		if err != nil {
			panic(err)
		}
		category := strings.SplitN(strings.TrimSuffix(path.Base(file), ".conf"), "-", 3)[2]
		for _, line := range strings.Split(strings.ReplaceAll(string(content), "\\\n", " "), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "SecRule ") {
				continue
			}
			i, m, l := id.FindStringSubmatch(line), message.FindStringSubmatch(line), level.FindStringSubmatch(line)
			if i == nil || m == nil || l == nil {
				continue
			}
			number, _ := strconv.Atoi(i[1])
			pl, _ := strconv.Atoi(l[1])
			rules = append(rules, Rule{number, strings.ReplaceAll(m[1], `\'`, "'"), category, pl})
			known[number] = true
		}
	}
	return rules, known
})

func Rules() []Rule {
	rules, _ := catalog()
	return rules
}
func Known(id int) bool {
	_, known := catalog()
	return known[id]
}
