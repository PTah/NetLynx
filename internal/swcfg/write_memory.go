package swcfg

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// WriteMemory сохраняет running-config в startup (write memory / write / save).
// Нужен после SNMP SET admin/description: SNMP меняет runtime/running, но не flash.
func WriteMemory(c Creds) error {
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return fmt.Errorf("нет host или ssh user")
	}
	v := DetectVendor(string(c.Vendor), c.SysDescr, c.Name)
	if v == VendorAuto {
		v = DetectVendor("", c.SysDescr, c.Name)
	}
	if v == VendorMikrotik {
		return writeMemoryMikrotik(c)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	enable := strings.TrimSpace(c.EnablePass)
	if enable == "" {
		enable = c.Password
	}
	client, err := DialSwitch(c)
	if err != nil {
		return err
	}
	defer client.Close()

	out, runErr := runWriteMemory(client, timeout, v, enable)
	if ierr := interpretWriteMemoryCLI(out); ierr != nil {
		if runErr != nil && !isSSHSessionEOF(runErr) {
			return fmt.Errorf("%w; %s", ierr, runErr)
		}
		return ierr
	}
	return nil
}

func runWriteMemory(client *ssh.Client, timeout time.Duration, v Vendor, enablePass string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	in, err := sess.StdinPipe()
	if err != nil {
		return "", err
	}
	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf
	if err := sess.RequestPty("xterm", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return "", err
	}
	if err := sess.Shell(); err != nil {
		return "", err
	}
	waitQuiet(&outBuf, 500*time.Millisecond, timeout)

	privCmd, _, writeCmd := confEnterCmds(v)
	send := func(s string, quiet time.Duration) error {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return err
		}
		waitQuiet(&outBuf, quiet, timeout)
		return nil
	}

	if strings.TrimSpace(privCmd) != "" {
		before := outBuf.Len()
		if err := send(privCmd, 800*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		chunk := strings.ToLower(outBuf.String()[before:])
		needPass := strings.Contains(chunk, "password") || strings.Contains(chunk, "assword:")
		alreadyPriv := strings.Contains(chunk, "#") && !needPass
		if needPass && !alreadyPriv {
			if err := send(enablePass, 700*time.Millisecond); err != nil {
				return outBuf.String(), err
			}
		}
	}

	if err := send(writeCmd, 2500*time.Millisecond); err != nil {
		return outBuf.String() + errBuf.String(), err
	}
	maybeConfirmWrite(&outBuf, send)

	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}

func interpretWriteMemoryCLI(out string) error {
	low := strings.ToLower(out)
	for _, bad := range []string{
		"invalid input detected",
		"incomplete command",
		"command not found",
		"unknown command",
		"% invalid",
		"permission denied",
		"authorization failed",
		"error:",
	} {
		if strings.Contains(low, bad) {
			return fmt.Errorf("cli write: %s — %s", bad, compactCLIErr(out))
		}
	}
	// Успех: явные маркеры или просто нет ошибок после write/save.
	okHints := []string{
		"write memory",
		"\nwrite",
		"ok",
		"success",
		"building configuration",
		"copied",
		"[ok]",
		"save",
	}
	for _, h := range okHints {
		if strings.Contains(low, h) {
			return nil
		}
	}
	// SNR/Edge часто просто возвращают prompt без текста — считаем ок, если не было bad.
	if strings.Contains(low, "#") || strings.Contains(low, ">") {
		return nil
	}
	return fmt.Errorf("cli write: нет подтверждения сохранения — %s", compactCLIErr(out))
}

func writeMemoryMikrotik(c Creds) error {
	// RouterOS: /export уже в Apply; для persistence достаточно /system backup или ничего
	// (изменения в /interface сразу в config). Явный save:
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client, err := DialSwitch(c)
	if err != nil {
		return err
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	var outBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &outBuf
	cmd := "/system backup save name=netlynx-autosave"
	if err := sess.Run(cmd); err != nil {
		// backup может быть запрещён — не считаем фатальным для RouterOS
		_ = err
	}
	return nil
}
