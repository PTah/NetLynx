package journallog

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultUnit = "NetLynx.service"

// Category — тематический фильтр по тексту сообщения (логика приложения).
type Category struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Match string `json:"-"` // подстрока в lower-case
}

var Categories = []Category{
	{ID: "poll", Label: "Опрос (poll)", Match: "poll"},
	{ID: "poe", Label: "PoE", Match: "poe"},
	{ID: "ssh", Label: "SSH", Match: "ssh"},
	{ID: "backup", Label: "Резервные копии", Match: "backup"},
	{ID: "auth", Label: "Авторизация", Match: "auth"},
	{ID: "trap", Label: "SNMP trap", Match: "trap"},
	{ID: "notify", Label: "Уведомления", Match: "notif"},
	{ID: "http", Label: "HTTP / API", Match: "http"},
	{ID: "offline", Label: "Онлайн / оффлайн", Match: "offline"},
}

// LevelFilter — уровни journald PRIORITY (0–7).
var LevelFilters = []struct {
	ID    string
	Label string
	MaxP  int // включаем priority <= MaxP (err=3 → 0..3)
}{
	{ID: "err", Label: "Ошибки", MaxP: 3},
	{ID: "warning", Label: "Предупреждения", MaxP: 4},
	{ID: "info", Label: "Инфо и ниже", MaxP: 6},
	{ID: "debug", Label: "Всё (вкл. debug)", MaxP: 7},
}

var (
	safeTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([ T]\d{2}:\d{2}(:\d{2})?)?$`)
	unitRe     = regexp.MustCompile(`^[A-Za-z0-9_.@+-]+\.service$`)
)

type Query struct {
	Unit       string
	Limit      int
	Since      string // YYYY-MM-DD[ HH:MM[:SS]]
	Until      string
	LevelID    string   // err|warning|info|debug|"" (без -p)
	Categories []string // ids from Categories; empty = all
	Follow     bool
}

type Line struct {
	Text string `json:"text"`
}

func normalizeUnit(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return DefaultUnit
	}
	if !unitRe.MatchString(u) {
		return DefaultUnit
	}
	return u
}

func ClampLimit(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func validateTime(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !safeTimeRe.MatchString(s) {
		return "", fmt.Errorf("неверный формат времени (ожидается YYYY-MM-DD или YYYY-MM-DD HH:MM)")
	}
	return strings.ReplaceAll(s, "T", " "), nil
}

func levelPriority(id string) (prio string, ok bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, lf := range LevelFilters {
		if lf.ID == id {
			// journalctl -p LEVEL: show that priority and higher severity (lower number)
			switch id {
			case "err":
				return "err", true
			case "warning":
				return "warning", true
			case "info":
				return "info", true
			case "debug":
				return "debug", true
			}
		}
	}
	return "", false
}

func categoryNeedles(ids []string) []string {
	want := make(map[string]struct{})
	for _, id := range ids {
		want[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	if len(want) == 0 {
		return nil
	}
	var out []string
	for _, c := range Categories {
		if _, ok := want[c.ID]; ok {
			out = append(out, c.Match)
		}
	}
	return out
}

func matchCategories(line string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	low := strings.ToLower(line)
	for _, n := range needles {
		if strings.Contains(low, n) {
			return true
		}
	}
	return false
}

func buildArgs(q Query) ([]string, error) {
	unit := normalizeUnit(q.Unit)
	args := []string{"--no-pager", "-o", "short-iso", "-u", unit}
	if q.Follow {
		args = append(args, "-f", "-n", strconv.Itoa(ClampLimit(q.Limit, 50, 200)))
	} else {
		lim := ClampLimit(q.Limit, 100, 2000)
		args = append(args, "-n", strconv.Itoa(lim))
	}
	since, err := validateTime(q.Since)
	if err != nil {
		return nil, err
	}
	until, err := validateTime(q.Until)
	if err != nil {
		return nil, err
	}
	if since != "" {
		args = append(args, "--since", since)
	}
	if until != "" {
		args = append(args, "--until", until)
	}
	if p, ok := levelPriority(q.LevelID); ok {
		args = append(args, "-p", p)
	}
	return args, nil
}

// ReadLines выполняет journalctl и возвращает строки (без follow).
func ReadLines(ctx context.Context, q Query) ([]string, error) {
	q.Follow = false
	want := ClampLimit(q.Limit, 100, 2000)
	if q.Since != "" || q.Until != "" {
		if q.Limit <= 0 {
			want = 500
		}
	}
	needles := categoryNeedles(q.Categories)
	fetch := want
	if len(needles) > 0 {
		fetch = ClampLimit(want*5, want, 5000)
	}
	q.Limit = fetch
	args, err := buildArgs(q)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("journalctl: %s", msg)
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		t := sc.Text()
		if !matchCategories(t, needles) {
			continue
		}
		lines = append(lines, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(lines) > want {
		lines = lines[len(lines)-want:]
	}
	return lines, nil
}

// Follow запускает journalctl -f и шлёт строки в ch до отмены ctx.
func Follow(ctx context.Context, q Query, ch chan<- string) error {
	q.Follow = true
	if q.Limit <= 0 {
		q.Limit = 50
	}
	args, err := buildArgs(q)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() {
		b, _ := io.ReadAll(stderr)
		waitErr := cmd.Wait()
		if waitErr != nil && ctx.Err() == nil {
			msg := strings.TrimSpace(string(b))
			if msg != "" {
				errCh <- fmt.Errorf("journalctl: %s", msg)
				return
			}
			errCh <- waitErr
			return
		}
		errCh <- nil
	}()

	needles := categoryNeedles(q.Categories)
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		t := sc.Text()
		if !matchCategories(t, needles) {
			continue
		}
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		case ch <- t:
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	select {
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		return nil
	}
}

func Available() map[string]any {
	cats := make([]map[string]string, 0, len(Categories))
	for _, c := range Categories {
		cats = append(cats, map[string]string{"id": c.ID, "label": c.Label})
	}
	levels := make([]map[string]string, 0, len(LevelFilters))
	for _, l := range LevelFilters {
		levels = append(levels, map[string]string{"id": l.ID, "label": l.Label})
	}
	return map[string]any{
		"unit":       DefaultUnit,
		"categories": cats,
		"levels":     levels,
		"limits":     []int{50, 100, 200, 500, 1000},
	}
}
