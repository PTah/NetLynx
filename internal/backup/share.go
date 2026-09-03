package backup

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
)

func WriteLocal(dir, filename string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := pathJoin(dir, filename)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return err
	}
	return nil
}

// CopyLocalFile копирует готовый файл в каталог бэкапов (без загрузки в RAM).
func CopyLocalFile(dir, filename, srcPath string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	p := pathJoin(dir, filename)
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func pathJoin(dir, file string) string {
	return strings.TrimRight(dir, `/\`) + string(os.PathSeparator) + file
}

type ShareSpec struct {
	Kind     string
	URL      string
	User     string
	Password string
	Domain   string
}

func DeliverShare(spec ShareSpec, filename string, data []byte) (err error) {
	return deliverShareReader(spec, filename, bytes.NewReader(data), int64(len(data)))
}

func DeliverShareFile(spec ShareSpec, filename, srcPath string) (err error) {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	return deliverShareReader(spec, filename, in, st.Size())
}

func deliverShareReader(spec ShareSpec, filename string, r io.Reader, size int64) (err error) {
	defer recoverSMB(&err)
	kind := strings.ToLower(strings.TrimSpace(spec.Kind))
	raw := strings.TrimSpace(spec.URL)
	if raw == "" {
		return fmt.Errorf("не задан путь шары")
	}
	if kind == "nfs" {
		if !looksLikeLocalPath(raw) {
			return fmt.Errorf("для NFS укажите уже смонтированный каталог на сервере NetLynx (например /mnt/nas/netlynx)")
		}
		return copyToLocalPath(raw, filename, r)
	}
	if kind != "smb" && looksLikeLocalPath(raw) {
		return copyToLocalPath(raw, filename, r)
	}
	host, share, rel, err := parseSMB(raw)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "445"), 20*time.Second)
	if err != nil {
		return fmt.Errorf("smb %s: %w", host, err)
	}
	defer conn.Close()
	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     spec.User,
			Password: spec.Password,
			Domain:   spec.Domain,
		},
	}
	s, err := d.Dial(conn)
	if err != nil {
		return fmt.Errorf("smb login: %w", err)
	}
	defer s.Logoff()
	fs, err := s.Mount(share)
	if err != nil {
		return fmt.Errorf("smb mount %s: %w", share, err)
	}
	defer fs.Umount()
	if rel != "" {
		if err := mkdirAllSMB(fs, rel); err != nil {
			return err
		}
	}
	full := filename
	if rel != "" {
		full = rel + "/" + filename
	}
	f, err := fs.Create(full)
	if err != nil {
		return fmt.Errorf("smb create: %w", err)
	}
	_, werr := io.Copy(f, r)
	cerr := f.Close()
	_ = size
	if werr != nil {
		return werr
	}
	return cerr
}

func copyToLocalPath(dir, filename string, r io.Reader) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := pathJoin(dir, filename)
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func recoverSMB(errp *error) {
	if rec := recover(); rec != nil {
		*errp = fmt.Errorf("smb: аварийный ответ шары (%v)", rec)
	}
}

func RotateShare(spec ShareSpec, retainDays int, now time.Time) (err error) {
	defer recoverSMB(&err)
	kind := strings.ToLower(strings.TrimSpace(spec.Kind))
	raw := strings.TrimSpace(spec.URL)
	if raw == "" {
		return nil
	}
	if kind == "nfs" || (kind != "smb" && looksLikeLocalPath(raw)) {
		if kind == "nfs" && !looksLikeLocalPath(raw) {
			return fmt.Errorf("для NFS укажите уже смонтированный каталог")
		}
		return RotateDir(raw, retainDays, now)
	}
	host, share, rel, err := parseSMB(raw)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "445"), 20*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     spec.User,
			Password: spec.Password,
			Domain:   spec.Domain,
		},
	}
	s, err := d.Dial(conn)
	if err != nil {
		return err
	}
	defer s.Logoff()
	fs, err := s.Mount(share)
	if err != nil {
		return err
	}
	defer fs.Umount()
	dir := "."
	if rel != "" {
		dir = rel
	}
	names, err := fs.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-time.Duration(retainDays) * 24 * time.Hour)
	for _, n := range names {
		name := n.Name()
		if !strings.HasPrefix(name, "netlynx-") || !strings.HasSuffix(name, ".zip") {
			continue
		}
		if n.ModTime().Before(cutoff) {
			p := name
			if rel != "" {
				p = rel + "/" + name
			}
			_ = fs.Remove(p)
		}
	}
	return nil
}

func looksLikeLocalPath(s string) bool {
	if strings.HasPrefix(s, "//") || strings.HasPrefix(s, `\\`) {
		return false
	}
	if strings.HasPrefix(s, "/") {
		return true
	}
	if len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	return false
}

func parseSMB(raw string) (host, share, rel string, err error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, `\`, "/")
	s = strings.TrimPrefix(s, "smb:")
	s = strings.TrimPrefix(s, "SMB:")
	s = strings.TrimPrefix(s, "//")
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("шара SMB: ожидается //хост/имя_шары[/каталог]")
	}
	host = parts[0]
	share = parts[1]
	if len(parts) > 2 {
		rel = path.Join(parts[2:]...)
		rel = strings.ReplaceAll(rel, "\\", "/")
	}
	return host, share, rel, nil
}

func mkdirAllSMB(fs *smb2.Share, rel string) error {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return nil
	}
	cur := ""
	for _, p := range strings.Split(rel, "/") {
		if p == "" {
			continue
		}
		if cur == "" {
			cur = p
		} else {
			cur += "/" + p
		}
		_ = fs.Mkdir(cur, 0o755)
	}
	return nil
}
