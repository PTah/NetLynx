package swcfg

import (
	"strings"
)

type DiffKind string

const (
	DiffEqual  DiffKind = "equal"
	DiffInsert DiffKind = "insert"
	DiffDelete DiffKind = "delete"
)

type DiffLine struct {
	Kind    DiffKind `json:"kind"`
	Text    string   `json:"text"`
	OldLine *int     `json:"old_line,omitempty"`
	NewLine *int     `json:"new_line,omitempty"`
}

// LineDiff строит построчный diff двух конфигов (LCS).
func LineDiff(oldText, newText string) []DiffLine {
	oldLines := splitConfigLines(oldText)
	newLines := splitConfigLines(newText)
	lcs := lcsTable(oldLines, newLines)
	return backtrackDiff(oldLines, newLines, lcs)
}

func splitConfigLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lcsTable(a, b []string) [][]int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	return dp
}

func backtrackDiff(a, b []string, dp [][]int) []DiffLine {
	i, j := len(a), len(b)
	var rev []DiffLine
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			o, n := i, j
			rev = append(rev, DiffLine{Kind: DiffEqual, Text: a[i-1], OldLine: &o, NewLine: &n})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			n := j
			rev = append(rev, DiffLine{Kind: DiffInsert, Text: b[j-1], NewLine: &n})
			j--
		default:
			o := i
			rev = append(rev, DiffLine{Kind: DiffDelete, Text: a[i-1], OldLine: &o})
			i--
		}
	}
	out := make([]DiffLine, len(rev))
	for k := range rev {
		out[len(rev)-1-k] = rev[k]
	}
	return out
}

type DiffStats struct {
	Equal  int `json:"equal"`
	Insert int `json:"insert"`
	Delete int `json:"delete"`
}

func DiffStatsFrom(lines []DiffLine) DiffStats {
	var st DiffStats
	for _, l := range lines {
		switch l.Kind {
		case DiffEqual:
			st.Equal++
		case DiffInsert:
			st.Insert++
		case DiffDelete:
			st.Delete++
		}
	}
	return st
}
