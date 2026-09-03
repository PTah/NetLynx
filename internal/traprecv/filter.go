package traprecv

import "strings"

// ParseIncludeLabels разбирает CSV trap_label; пустая строка = без фильтра (все типы).
func ParseIncludeLabels(csv string) map[string]struct{} {
	raw := strings.TrimSpace(csv)
	if raw == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		l := strings.TrimSpace(part)
		if l == "" {
			continue
		}
		out[l] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// JoinIncludeLabels собирает CSV из списка (без дубликатов, порядок options).
func JoinIncludeLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(labels))
	ordered := make([]string, 0, len(labels))
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		ordered = append(ordered, l)
	}
	return strings.Join(ordered, ",")
}

// LabelMatchesInclude true, если фильтр выключен или label в списке.
func LabelMatchesInclude(label string, include map[string]struct{}) bool {
	if len(include) == 0 {
		return true
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "SNMP trap"
	}
	_, ok := include[label]
	return ok
}
