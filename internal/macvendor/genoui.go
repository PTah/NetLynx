//go:build ignore

// Сборка компактной базы OUI из CSV IEEE Registration Authority.
//
//	go run internal/macvendor/genoui.go -out internal/macvendor/oui.tsv.gz path/to/oui.csv [mam.csv oui36.csv cid.csv]
package main

import (
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"
)

func main() {
	out := flag.String("out", "internal/macvendor/oui.tsv.gz", "gzip TSV: PREFIX\\tORG")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: go run internal/macvendor/genoui.go -out oui.tsv.gz oui.csv [mam.csv oui36.csv cid.csv]")
		os.Exit(2)
	}
	entries := map[string]string{}
	for _, path := range flag.Args() {
		n, err := mergeCSV(path, entries)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("%s: +%d\n", path, n)
	}
	if err := writeGZ(*out, entries); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d prefixes)\n", *out, len(entries))
}

func mergeCSV(path string, dst map[string]string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	added := 0
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return added, err
		}
		if first {
			first = false
			if len(rec) > 0 && strings.EqualFold(strings.TrimSpace(rec[0]), "Registry") {
				continue
			}
		}
		if len(rec) < 3 {
			continue
		}
		prefix := normalizePrefix(rec[1])
		org := cleanOrg(rec[2])
		if prefix == "" || org == "" {
			continue
		}
		if _, ok := dst[prefix]; !ok {
			added++
		}
		dst[prefix] = org
	}
	return added, nil
}

func normalizePrefix(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r - ('a' - 'A'))
		case r >= 'A' && r <= 'F':
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) != 6 && len(s) != 7 && len(s) != 9 {
		return ""
	}
	return s
}

func cleanOrg(raw string) string {
	s := html.UnescapeString(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	if low == "private" || low == "ieee registration authority" {
		return ""
	}
	return s
}

func writeGZ(path string, entries map[string]string) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(zw, "# IEEE OUI/CID/MA-M/MA-S (public registry). PREFIX<TAB>organization\n"); err != nil {
		_ = zw.Close()
		return err
	}
	for _, k := range keys {
		org := entries[k]
		if strings.IndexFunc(org, unicode.IsControl) >= 0 {
			org = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return -1
				}
				return r
			}, org)
		}
		if _, err := fmt.Fprintf(zw, "%s\t%s\n", k, org); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}
