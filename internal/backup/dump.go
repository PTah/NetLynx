package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DumpDatabase снимает дамп в память (legacy/tests). На prod используйте DumpDatabaseFile.
func DumpDatabase(ctx context.Context, databaseURL string, note func(string)) ([]byte, error) {
	path, cleanup, err := DumpDatabaseFile(ctx, databaseURL, note)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return os.ReadFile(path)
}

// DumpDatabaseFile пишет дамп во временный файл (не держит SQL в RAM процесса).
func DumpDatabaseFile(ctx context.Context, databaseURL string, note func(string)) (path string, cleanup func(), err error) {
	if note == nil {
		note = func(string) {}
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", nil, fmt.Errorf("DATABASE_URL: %w", err)
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	db := strings.TrimPrefix(u.Path, "/")
	if i := strings.Index(db, "/"); i >= 0 {
		db = db[:i]
	}
	if db == "" {
		return "", nil, fmt.Errorf("DATABASE_URL: нет имени базы")
	}

	tmp, err := os.CreateTemp("", "netlynx-dump-*.sql")
	if err != nil {
		return "", nil, err
	}
	dumpPath := tmp.Name()
	cleanup = func() {
		_ = tmp.Close()
		_ = os.Remove(dumpPath)
	}

	var errs []string
	bin := findPgDump()
	if bin != "" {
		note("дамп БД: " + bin + " → " + host + ":" + port + "/" + db)
		if err := runPgDumpBinToFile(ctx, bin, user, pass, host, port, db, tmp); err != nil {
			note("pg_dump на хосте не вышел: " + err.Error())
			errs = append(errs, "pg_dump: "+err.Error())
		} else if st, statErr := tmp.Stat(); statErr == nil && st.Size() > 0 {
			_ = tmp.Close()
			note(fmt.Sprintf("дамп БД готов (%d КБ)", st.Size()/1024))
			return dumpPath, cleanup, nil
		} else {
			errs = append(errs, "pg_dump: пустой вывод")
		}
		_ = tmp.Truncate(0)
		_, _ = tmp.Seek(0, io.SeekStart)
	} else {
		note("на сервере нет программы pg_dump")
		errs = append(errs, "pg_dump не найден в PATH (пакет postgresql-client)")
	}

	if dockerAvailable() {
		note("дамп БД через docker")
		if err := runDockerPgDumpToFile(ctx, user, db, tmp); err != nil {
			note("docker pg_dump не вышел: " + err.Error())
			errs = append(errs, "docker: "+err.Error())
		} else if st, statErr := tmp.Stat(); statErr == nil && st.Size() > 0 {
			_ = tmp.Close()
			note(fmt.Sprintf("дамп БД из docker готов (%d КБ)", st.Size()/1024))
			return dumpPath, cleanup, nil
		} else {
			errs = append(errs, "docker pg_dump: пустой вывод")
		}
		_ = tmp.Truncate(0)
		_, _ = tmp.Seek(0, io.SeekStart)
	} else {
		note("docker недоступен пользователю сервиса (нет права на docker.sock)")
	}

	note("дамп БД через подключение приложения (без pg_dump)")
	if err := dumpLogicalToFile(ctx, databaseURL, note, tmp); err != nil {
		note("логический дамп не вышел: " + err.Error())
		errs = append(errs, "logical: "+err.Error())
	} else if st, statErr := tmp.Stat(); statErr == nil && st.Size() > 0 {
		_ = tmp.Close()
		note(fmt.Sprintf("логический дамп БД готов (%d КБ)", st.Size()/1024))
		return dumpPath, cleanup, nil
	} else {
		errs = append(errs, "logical: пустой вывод")
	}

	cleanup()
	return "", nil, fmt.Errorf("%s", strings.Join(errs, "; "))
}

func findPgDump() string {
	if p, err := exec.LookPath("pg_dump"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	for _, p := range []string{
		"/opt/netlynx/pg_dump",
		"/usr/bin/pg_dump",
		"/usr/lib/postgresql/17/bin/pg_dump",
		"/usr/lib/postgresql/16/bin/pg_dump",
		"/usr/lib/postgresql/15/bin/pg_dump",
		"/usr/lib/postgresql/14/bin/pg_dump",
	} {
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

type pgConn struct {
	User, Pass, Host, Port, DB string
}

func runPgDumpBinToFile(ctx context.Context, bin, user, pass, host, port, db string, out io.Writer) error {
	args := []string{"--no-owner", "--no-acl", "-h", host, "-p", port, "-U", user, "-d", db}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+pass, "PGCLIENTENCODING=UTF8")
	var stderr bytes.Buffer
	cmd.Stdout = out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func runPgDumpBin(ctx context.Context, bin, user, pass, host, port, db string) ([]byte, error) {
	var stdout bytes.Buffer
	if err := runPgDumpBinToFile(ctx, bin, user, pass, host, port, db, &stdout); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func findPsql() string {
	if p, err := exec.LookPath("psql"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	for _, p := range []string{
		"/usr/bin/psql",
		"/usr/lib/postgresql/17/bin/psql",
		"/usr/lib/postgresql/16/bin/psql",
		"/usr/lib/postgresql/15/bin/psql",
		"/usr/lib/postgresql/14/bin/psql",
	} {
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	host := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	if host == "" || strings.HasPrefix(host, "unix://") {
		sock := "/var/run/docker.sock"
		if strings.HasPrefix(host, "unix://") {
			sock = strings.TrimPrefix(host, "unix://")
		}
		f, err := os.OpenFile(sock, os.O_RDWR, 0)
		if err != nil {
			return false
		}
		_ = f.Close()
		return true
	}
	return true
}

func dirAccessible(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func runDockerPgDumpToFile(ctx context.Context, user, db string, out io.Writer) error {
	repo := strings.TrimSpace(readRepoPath())
	var errs []string
	if repo != "" && dirAccessible(repo) {
		cmd := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "postgres", "pg_dump", "-U", user, db)
		cmd.Dir = repo
		if err := runCmdToWriter(cmd, out); err == nil {
			return nil
		} else {
			errs = append(errs, "compose: "+err.Error())
		}
	} else if repo != "" {
		errs = append(errs, "каталог compose недоступен: "+repo)
	} else {
		errs = append(errs, "нет /opt/netlynx/repo.path")
	}
	for _, cname := range []string{"netlynx-postgres-1", "netlynx_postgres_1", "invetor-postgres-1", "invetor_postgres_1"} {
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", cname, "pg_dump", "-U", user, db)
		if err := runCmdToWriter(cmd, out); err == nil {
			return nil
		} else {
			errs = append(errs, cname+": "+err.Error())
		}
	}
	if len(errs) == 0 {
		return fmt.Errorf("не удалось снять дамп через docker")
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

func runDockerPgDump(ctx context.Context, user, db string) ([]byte, error) {
	var buf bytes.Buffer
	if err := runDockerPgDumpToFile(ctx, user, db, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func runCmdToWriter(cmd *exec.Cmd, out io.Writer) error {
	var stderr bytes.Buffer
	cmd.Stdout = out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func runCmdCapture(cmd *exec.Cmd) ([]byte, error) {
	var stdout bytes.Buffer
	if err := runCmdToWriter(cmd, &stdout); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func readRepoPath() string {
	b, err := os.ReadFile("/opt/netlynx/repo.path")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func ReadEnvFile() (name string, data []byte, err error) {
	candidates := []string{"/etc/netlynx/netlynx.env"}
	if wd, e := os.Getwd(); e == nil {
		candidates = append(candidates, filepath.Join(wd, ".env"))
	}
	var last error
	for _, p := range candidates {
		b, e := os.ReadFile(p)
		if e == nil {
			return filepath.Base(p), b, nil
		}
		last = e
	}
	if last == nil {
		last = fmt.Errorf("env file not found")
	}
	return "", nil, last
}

func TimestampName(t time.Time) string {
	// Часовой пояс самого t, не машины: иначе CI в UTC ломает имена бэкапов.
	return t.Format("20060102-1504")
}
