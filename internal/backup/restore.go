package backup

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ArchiveInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

var archiveNameRe = regexp.MustCompile(`(?i)^netlynx-[0-9a-z._-]+\.zip$`)

func ListLocalArchives(dir string) ([]ArchiveInfo, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("не задан каталог бэкапов")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ArchiveInfo{}, nil
		}
		return nil, err
	}
	var out []ArchiveInfo
	for _, e := range ents {
		if e.IsDir() || !archiveNameRe.MatchString(e.Name()) {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, ArchiveInfo{Name: e.Name(), Size: st.Size(), ModTime: st.ModTime()})
	}
	return out, nil
}

func SafeArchivePath(dir, name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if !archiveNameRe.MatchString(base) {
		return "", fmt.Errorf("ожидается файл netlynx-*.zip")
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dirAbs, base)
	pAbs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	sep := string(os.PathSeparator)
	if pAbs != filepath.Join(dirAbs, base) && !strings.HasPrefix(pAbs, dirAbs+sep) {
		return "", fmt.Errorf("файл вне каталога бэкапов")
	}
	st, err := os.Stat(pAbs)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("это каталог, а не ZIP")
	}
	return pAbs, nil
}

func (r *Runner) ListArchives(ctx context.Context) ([]ArchiveInfo, error) {
	bs, err := r.st.GetBackupSettings(ctx)
	if err != nil {
		return nil, err
	}
	return ListLocalArchives(bs.LocalDir)
}

func (r *Runner) VerifyArchive(ctx context.Context, zipPath string) error {
	if !r.mu.TryLock() {
		return fmt.Errorf("бэкап или восстановление уже выполняется")
	}
	defer r.mu.Unlock()
	r.startProgress()
	defer r.stopProgress()
	r.note(ctx, "проверка дампа (временная БД, рабочая не затрагивается)")
	_ = r.st.SetBackupRunResult(ctx, "running", "", r.logText())
	err := r.verifyLocked(ctx, zipPath)
	if err != nil {
		r.note(ctx, "ошибка: "+err.Error())
		_ = r.st.SetBackupRunResult(ctx, "fail", err.Error(), r.logText())
		return err
	}
	r.note(ctx, "проверка успешна, временная БД удалена")
	_ = r.st.SetBackupRunResult(ctx, "ok", "", r.logText())
	return nil
}

func (r *Runner) ImportArchive(ctx context.Context, zipPath string) error {
	if !r.mu.TryLock() {
		return fmt.Errorf("бэкап или восстановление уже выполняется")
	}
	defer r.mu.Unlock()
	r.startProgress()
	defer r.stopProgress()
	r.note(ctx, "импорт дампа в рабочую БД (только пустая система)")
	_ = r.st.SetBackupRunResult(ctx, "running", "", r.logText())
	err := r.importLocked(ctx, zipPath)
	if err != nil {
		r.note(ctx, "ошибка: "+err.Error())
		_ = r.st.SetBackupRunResult(ctx, "fail", err.Error(), r.logText())
		return err
	}
	r.note(ctx, "импорт готов — перезапустите службу: sudo systemctl restart NetLynx.service")
	_ = r.st.SetBackupRunResult(ctx, "ok", "", r.logText())
	return nil
}

func (r *Runner) verifyLocked(ctx context.Context, zipPath string) error {
	pg, err := parsePGConn(r.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	psql := findPsql()
	if psql == "" {
		return fmt.Errorf("нет psql (пакет postgresql-client)")
	}
	sqlFile, man, cleanup, err := extractDumpSQL(zipPath)
	if err != nil {
		return err
	}
	defer cleanup()
	if man.CreatedAt != "" {
		r.note(ctx, "архив от "+man.CreatedAt+" статус "+man.Status)
	}
	tmp := fmt.Sprintf("invetor_rv_%d", time.Now().UTC().Unix())
	ident, err := pgSafeIdent(tmp)
	if err != nil {
		return err
	}
	r.note(ctx, "создаю временную БД "+tmp+" (рабочая "+pg.DB+" не меняется)")
	if err := psqlExec(ctx, psql, pg, pg.DB, "CREATE DATABASE "+ident); err != nil {
		return fmt.Errorf("CREATE DATABASE (нужны права CREATEDB у пользователя DATABASE_URL): %w", err)
	}
	defer func() {
		_ = psqlExec(context.Background(), psql, pg, pg.DB, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)")
	}()
	r.note(ctx, "заливаю дамп во временную БД")
	if err := psqlFile(ctx, psql, pg, tmp, sqlFile); err != nil {
		return fmt.Errorf("restore temp: %w", err)
	}
	return r.reportDumpStats(ctx, psql, pg, tmp)
}

func (r *Runner) importLocked(ctx context.Context, zipPath string) error {
	n, err := r.st.CountDevices(ctx)
	if err != nil {
		return fmt.Errorf("проверка рабочей БД: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("рабочая база не пуста (%d узлов). Импорт только на чистую систему. Проверку дампа можно сделать без импорта", n)
	}
	pg, err := parsePGConn(r.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	psql := findPsql()
	if psql == "" {
		return fmt.Errorf("нет psql (пакет postgresql-client)")
	}
	sqlFile, man, cleanup, err := extractDumpSQL(zipPath)
	if err != nil {
		return err
	}
	defer cleanup()
	if man.CreatedAt != "" {
		r.note(ctx, "архив от "+man.CreatedAt)
	}
	tmp := fmt.Sprintf("invetor_rv_%d", time.Now().UTC().Unix())
	ident, err := pgSafeIdent(tmp)
	if err != nil {
		return err
	}
	r.note(ctx, "сначала заливаю дамп во временную БД "+tmp)
	if err := psqlExec(ctx, psql, pg, pg.DB, "CREATE DATABASE "+ident); err != nil {
		return fmt.Errorf("CREATE DATABASE (нужны права CREATEDB у пользователя DATABASE_URL): %w", err)
	}
	keepTmp := false
	defer func() {
		if keepTmp {
			r.note(ctx, "временная БД "+tmp+" сохранена — восстановите вручную (pg_dump) и удалите её")
			return
		}
		_ = psqlExec(context.Background(), psql, pg, pg.DB, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)")
	}()
	if err := psqlFile(ctx, psql, pg, tmp, sqlFile); err != nil {
		return fmt.Errorf("пробный restore во временную БД: %w", err)
	}
	r.note(ctx, "пробный restore успешен — очищаю public и заливаю в рабочую БД")
	if err := psqlExec(ctx, psql, pg, pg.DB, "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO PUBLIC"); err != nil {
		return fmt.Errorf("очистка схемы: %w", err)
	}
	if err := psqlFile(ctx, psql, pg, pg.DB, sqlFile); err != nil {
		keepTmp = true
		return fmt.Errorf("restore рабочей БД (схема уже очищена, дамп есть в %s): %w", tmp, err)
	}
	return r.reportDumpStats(ctx, psql, pg, pg.DB)
}

func (r *Runner) reportDumpStats(ctx context.Context, psql string, pg pgConn, db string) error {
	tables := []string{"devices", "device_interfaces", "events", "schema_migrations", "users"}
	var parts []string
	for _, t := range tables {
		ident, err := pgSafeIdent(t)
		if err != nil {
			continue
		}
		q := fmt.Sprintf(`SELECT COUNT(*)::text FROM information_schema.tables WHERE table_schema='public' AND table_name='%s'`, t)
		has, err := psqlScalar(ctx, psql, pg, db, q)
		if err != nil || strings.TrimSpace(has) != "1" {
			parts = append(parts, t+"=нет")
			continue
		}
		cnt, err := psqlScalar(ctx, psql, pg, db, "SELECT COUNT(*)::text FROM "+ident)
		if err != nil {
			parts = append(parts, t+"=?")
			continue
		}
		parts = append(parts, t+"="+strings.TrimSpace(cnt))
	}
	r.note(ctx, "содержимое: "+strings.Join(parts, ", "))
	return nil
}

func extractDumpSQL(zipPath string) (sqlPath string, man Manifest, cleanup func(), err error) {
	cleanup = func() {}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", man, cleanup, fmt.Errorf("zip: %w", err)
	}
	defer zr.Close()
	var dumpF, manF *zip.File
	for i := range zr.File {
		name := zr.File[i].Name
		base := filepath.Base(name)
		switch base {
		case "db.sql.gz":
			dumpF = zr.File[i]
		case "manifest.json":
			manF = zr.File[i]
		}
	}
	if dumpF == nil {
		return "", man, cleanup, fmt.Errorf("в ZIP нет db.sql.gz — это не архив NetLynx")
	}
	if manF != nil {
		rc, e := manF.Open()
		if e == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
			_ = rc.Close()
			_ = json.Unmarshal(b, &man)
		}
	}
	tmp, err := os.MkdirTemp("", "invetor-restore-*")
	if err != nil {
		return "", man, cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	sqlPath = filepath.Join(tmp, "db.sql")
	rc, err := dumpF.Open()
	if err != nil {
		cleanup()
		return "", man, func() {}, err
	}
	defer rc.Close()
	gr, err := gzip.NewReader(rc)
	if err != nil {
		cleanup()
		return "", man, func() {}, fmt.Errorf("db.sql.gz: %w", err)
	}
	defer gr.Close()
	out, err := os.Create(sqlPath)
	if err != nil {
		cleanup()
		return "", man, func() {}, err
	}
	if err := stripPsqlConnect(gr, out); err != nil {
		_ = out.Close()
		cleanup()
		return "", man, func() {}, err
	}
	if err := out.Close(); err != nil {
		cleanup()
		return "", man, func() {}, err
	}
	return sqlPath, man, cleanup, nil
}

var pgIdentRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func pgSafeIdent(name string) (string, error) {
	if !pgIdentRe.MatchString(name) {
		return "", fmt.Errorf("небезопасное имя БД")
	}
	return `"` + name + `"`, nil
}

// keepPsqlMetaLine: COPY-терминатор и служебные маркеры pg_dump 17+.
func keepPsqlMetaLine(trim string) bool {
	if trim == `\.` {
		return true
	}
	low := strings.ToLower(trim)
	return strings.HasPrefix(low, `\restrict`) || strings.HasPrefix(low, `\unrestrict`) || strings.HasPrefix(low, `\encoding`)
}

// stripPsqlConnect убирает psql-метакоманды (\connect, \!, \i, \copy …).
// `\.` (конец COPY) оставляем. -X в psql отключает только .psqlrc, не \! в файле.
func stripPsqlConnect(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, `\`) && !keepPsqlMetaLine(trim) {
			continue
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	return sc.Err()
}

func parsePGConn(databaseURL string) (pgConn, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return pgConn{}, fmt.Errorf("DATABASE_URL: %w", err)
	}
	c := pgConn{User: u.User.Username(), Host: u.Hostname(), Port: u.Port(), DB: strings.TrimPrefix(u.Path, "/")}
	c.Pass, _ = u.User.Password()
	if c.Port == "" {
		c.Port = "5432"
	}
	if i := strings.Index(c.DB, "/"); i >= 0 {
		c.DB = c.DB[:i]
	}
	if c.DB == "" {
		return pgConn{}, fmt.Errorf("DATABASE_URL: нет имени базы")
	}
	return c, nil
}

func psqlEnv(c pgConn) []string {
	return append(os.Environ(), "PGPASSWORD="+c.Pass, "PGCLIENTENCODING=UTF8")
}

func psqlExec(ctx context.Context, bin string, c pgConn, db, sql string) error {
	cmd := exec.CommandContext(ctx, bin, "-X", "-h", c.Host, "-p", c.Port, "-U", c.User, "-d", db, "-v", "ON_ERROR_STOP=1", "-c", sql)
	cmd.Env = psqlEnv(c)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func psqlFile(ctx context.Context, bin string, c pgConn, db, sqlFile string) error {
	cmd := exec.CommandContext(ctx, bin, "-X", "-h", c.Host, "-p", c.Port, "-U", c.User, "-d", db, "-v", "ON_ERROR_STOP=1", "-f", sqlFile)
	cmd.Env = psqlEnv(c)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func psqlScalar(ctx context.Context, bin string, c pgConn, db, sql string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "-X", "-h", c.Host, "-p", c.Port, "-U", c.User, "-d", db, "-t", "-A", "-c", sql)
	cmd.Env = psqlEnv(c)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}