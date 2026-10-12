package store

import (
	"fmt"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/secrets"
)

// SetSecretsBox задаёт AES-GCM box для at-rest секретов (nil = plaintext compat).
func (s *Store) SetSecretsBox(b *secrets.Box) {
	if s == nil {
		return
	}
	s.sec = b
}

func (s *Store) secretsBox() *secrets.Box {
	if s == nil {
		return nil
	}
	return s.sec
}

func (s *Store) sealPtr(p *string) (*string, error) {
	out, err := secrets.MaybeSealPtr(s.secretsBox(), p)
	if err != nil {
		return nil, fmt.Errorf("seal secret: %w", err)
	}
	return out, nil
}

func (s *Store) openPtr(p *string) (*string, error) {
	out, err := secrets.MustOpenPtr(s.secretsBox(), p)
	if err != nil {
		return nil, fmt.Errorf("open secret: %w", err)
	}
	return out, nil
}

func (s *Store) openPollDeviceSecrets(d *PollDevice) error {
	if d == nil {
		return nil
	}
	var err error
	if d.Community, err = s.openPtr(d.Community); err != nil {
		return err
	}
	if d.V3AuthPass, err = s.openPtr(d.V3AuthPass); err != nil {
		return err
	}
	if d.V3PrivPass, err = s.openPtr(d.V3PrivPass); err != nil {
		return err
	}
	return nil
}

func (s *Store) openDeviceModelSecrets(d *models.Device) error {
	if d == nil {
		return nil
	}
	var err error
	if d.Community, err = s.openPtr(d.Community); err != nil {
		return err
	}
	if d.V3AuthPass, err = s.openPtr(d.V3AuthPass); err != nil {
		return err
	}
	if d.V3PrivPass, err = s.openPtr(d.V3PrivPass); err != nil {
		return err
	}
	if d.SSHPassword, err = s.openPtr(d.SSHPassword); err != nil {
		return err
	}
	if d.SSHEnablePassword, err = s.openPtr(d.SSHEnablePassword); err != nil {
		return err
	}
	return nil
}

func (s *Store) sealCreateDeviceSecrets(in *CreateDeviceInput) error {
	if in == nil {
		return nil
	}
	var err error
	if in.Community, err = s.sealPtr(in.Community); err != nil {
		return err
	}
	if in.V3AuthPass, err = s.sealPtr(in.V3AuthPass); err != nil {
		return err
	}
	if in.V3PrivPass, err = s.sealPtr(in.V3PrivPass); err != nil {
		return err
	}
	return nil
}
