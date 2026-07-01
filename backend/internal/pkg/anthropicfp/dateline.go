// Package anthropicfp provides helpers for suppressing client-side
// fingerprints visible to upstream Anthropic through forwarded requests.
package anthropicfp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var (
	datelineRegexHyphen = regexp.MustCompile(`Today(['’ʼʹ])s date is (\d{4})-(\d{2})-(\d{2})\.`)
	datelineRegexSlash  = regexp.MustCompile(`Today(['’ʼʹ])s date is (\d{4})/(\d{2})/(\d{2})\.`)
	systemReminderRegex = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
)

// DatelineHit records what a single rewrite normalized.
type DatelineHit struct {
	ApostropheVariant string
	DateSeparator     string
}

func mustWriteString(b *strings.Builder, s string) {
	_, _ = b.WriteString(s)
}

func canonicalize(year, month, day string) string {
	return fmt.Sprintf("Today's date is %s-%s-%s.", year, month, day)
}

func apostropheVariant(r rune) string {
	switch r {
	case '’':
		return "u2019"
	case 'ʼ':
		return "u02bc"
	case 'ʹ':
		return "u02b9"
	default:
		return "ascii"
	}
}

type datelineMatch struct {
	start, end       int
	apoRune          rune
	sep              string
	year, month, day string
}

func collectMatches(text string, re *regexp.Regexp, sep string) []datelineMatch {
	locs := re.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([]datelineMatch, 0, len(locs))
	for _, m := range locs {
		var apoRune rune
		for _, r := range text[m[2]:m[3]] {
			apoRune = r
			break
		}
		out = append(out, datelineMatch{
			start:   m[0],
			end:     m[1],
			apoRune: apoRune,
			sep:     sep,
			year:    text[m[4]:m[5]],
			month:   text[m[6]:m[7]],
			day:     text[m[8]:m[9]],
		})
	}
	return out
}

// NormalizeText replaces fingerprinted dateline sentences with canonical
// ASCII apostrophe and hyphen date separators.
func NormalizeText(text string) (string, []DatelineHit) {
	if !strings.Contains(text, "date is ") {
		return text, nil
	}
	matches := collectMatches(text, datelineRegexHyphen, "-")
	matches = append(matches, collectMatches(text, datelineRegexSlash, "/")...)
	if len(matches) == 0 {
		return text, nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })

	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	hits := make([]DatelineHit, 0, len(matches))
	changed := false
	for _, m := range matches {
		full := text[m.start:m.end]
		canonical := canonicalize(m.year, m.month, m.day)
		if canonical == full {
			continue
		}
		mustWriteString(&b, text[prev:m.start])
		mustWriteString(&b, canonical)
		prev = m.end
		changed = true
		hits = append(hits, DatelineHit{
			ApostropheVariant: apostropheVariant(m.apoRune),
			DateSeparator:     m.sep,
		})
	}
	if !changed {
		return text, nil
	}
	mustWriteString(&b, text[prev:])
	return b.String(), hits
}

func normalizeSystemReminderScopedText(text string) (string, []DatelineHit) {
	if !strings.Contains(text, "<system-reminder>") {
		return text, nil
	}
	locs := systemReminderRegex.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return text, nil
	}
	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	var hits []DatelineHit
	changed := false
	for _, loc := range locs {
		mustWriteString(&b, text[prev:loc[0]])
		block := text[loc[0]:loc[1]]
		normalized, blockHits := NormalizeText(block)
		if normalized != block {
			changed = true
		}
		mustWriteString(&b, normalized)
		hits = append(hits, blockHits...)
		prev = loc[1]
	}
	if !changed {
		return text, nil
	}
	mustWriteString(&b, text[prev:])
	return b.String(), hits
}

// NormalizeDateline rewrites client dateline fingerprints in Anthropic
// /v1/messages request bodies. It scans top-level system text and
// <system-reminder> blocks inside messages, leaving free user prose untouched.
func NormalizeDateline(body []byte) ([]byte, []DatelineHit, bool) {
	if len(body) == 0 {
		return body, nil, false
	}
	out := body
	var hits []DatelineHit
	changed := false

	sys := gjson.GetBytes(out, "system")
	if sys.Exists() {
		switch {
		case sys.Type == gjson.String:
			normalized, sysHits := NormalizeText(sys.String())
			if normalized != sys.String() {
				if next, err := sjson.SetBytes(out, "system", normalized); err == nil {
					out = next
					changed = true
					hits = append(hits, sysHits...)
				}
			}
		case sys.IsArray():
			idx := 0
			sys.ForEach(func(_, item gjson.Result) bool {
				if item.Get("type").String() == "text" {
					t := item.Get("text")
					if t.Exists() && t.Type == gjson.String {
						normalized, textHits := NormalizeText(t.String())
						if normalized != t.String() {
							path := fmt.Sprintf("system.%d.text", idx)
							if next, err := sjson.SetBytes(out, path, normalized); err == nil {
								out = next
								changed = true
								hits = append(hits, textHits...)
							}
						}
					}
				}
				idx++
				return true
			})
		}
	}

	messages := gjson.GetBytes(out, "messages")
	if messages.IsArray() {
		msgIdx := -1
		messages.ForEach(func(_, msg gjson.Result) bool {
			msgIdx++
			content := msg.Get("content")
			if !content.Exists() {
				return true
			}
			switch {
			case content.Type == gjson.String:
				normalized, contentHits := normalizeSystemReminderScopedText(content.String())
				if normalized != content.String() {
					path := fmt.Sprintf("messages.%d.content", msgIdx)
					if next, err := sjson.SetBytes(out, path, normalized); err == nil {
						out = next
						changed = true
						hits = append(hits, contentHits...)
					}
				}
			case content.IsArray():
				contentIdx := -1
				content.ForEach(func(_, block gjson.Result) bool {
					contentIdx++
					if block.Get("type").String() != "text" {
						return true
					}
					t := block.Get("text")
					if !t.Exists() || t.Type != gjson.String {
						return true
					}
					normalized, textHits := normalizeSystemReminderScopedText(t.String())
					if normalized != t.String() {
						path := fmt.Sprintf("messages.%d.content.%d.text", msgIdx, contentIdx)
						if next, err := sjson.SetBytes(out, path, normalized); err == nil {
							out = next
							changed = true
							hits = append(hits, textHits...)
						}
					}
					return true
				})
			}
			return true
		})
	}

	if !changed {
		return body, nil, false
	}
	return out, hits, true
}
