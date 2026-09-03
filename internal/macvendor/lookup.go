// Package macvendor определяет производителя по OUI MAC (реестр IEEE, без сетевых запросов).
//
// База: internal/macvendor/oui.tsv.gz (MA-L / MA-M / MA-S / CID).
// Обновление из корня репозитория:
//
//	go run internal/macvendor/genoui.go -out internal/macvendor/oui.tsv.gz oui.csv mam.csv oui36.csv cid.csv
package macvendor

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

//go:embed oui.tsv.gz
var ouiFS embed.FS

var (
	loadOnce sync.Once
	byPrefix map[string]string
)

// Lookup возвращает название организации по MAC (aa:bb:cc:… или без разделителей).
// Пустая строка, если префикс неизвестен или MAC слишком короткий.
func Lookup(mac string) string {
	h := hexDigits(mac)
	if len(h) < 6 {
		return ""
	}
	loadOnce.Do(load)
	if byPrefix == nil {
		return ""
	}
	if len(h) >= 9 {
		if v := byPrefix[h[:9]]; v != "" {
			return v
		}
	}
	if len(h) >= 7 {
		if v := byPrefix[h[:7]]; v != "" {
			return v
		}
	}
	if v := byPrefix[h[:6]]; v != "" {
		return v
	}
	if masked := maskOUI24(h[:6]); masked != h[:6] {
		return byPrefix[masked]
	}
	return ""
}

func load() {
	raw, err := ouiFS.ReadFile("oui.tsv.gz")
	if err != nil {
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return
	}
	defer zr.Close()
	m := make(map[string]string, 40000)
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		prefix, org, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		org = strings.TrimSpace(org)
		if (len(prefix) != 6 && len(prefix) != 7 && len(prefix) != 9) || org == "" {
			continue
		}
		m[prefix] = org
	}
	if sc.Err() != nil {
		return
	}
	byPrefix = m
}

func hexDigits(raw string) string {
	var b strings.Builder
	b.Grow(12)
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= '0' && c <= '9':
			b.WriteByte(c)
		case c >= 'a' && c <= 'f':
			b.WriteByte(c)
		case c >= 'A' && c <= 'F':
			b.WriteByte(c + ('a' - 'A'))
		}
	}
	return b.String()
}

// maskOUI24 снимает I/G и U/L биты первого октета (multicast / locally administered).
func maskOUI24(h6 string) string {
	if len(h6) != 6 {
		return h6
	}
	n, err := strconv.ParseUint(h6[:2], 16, 8)
	if err != nil {
		return h6
	}
	n &= 0xFC
	return fmt.Sprintf("%02x%s", n, h6[2:])
}
