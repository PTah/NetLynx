package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Категории узлов (список «Узлы» / карточка устройства).
const (
	DeviceCategorySwitch   = "switch"
	DeviceCategoryRouter   = "router"
	DeviceCategoryServer   = "server"
	DeviceCategoryComputer = "computer"
	DeviceCategoryPhone    = "phone"
	DeviceCategoryMFU      = "mfu"
	DeviceCategoryCamera   = "camera"
	DeviceCategoryAP       = "ap"
	DeviceCategoryOther    = "other"
)

var (
	categoryIDRe    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	categoryColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// DeviceCategoryDef строка справочника типов узлов.
type DeviceCategoryDef struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Color     string    `json:"color"`
	Blink     bool      `json:"blink"`
	Builtin   bool      `json:"builtin"`
	SortOrder int       `json:"sort_order"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ValidCategoryID(raw string) bool {
	return categoryIDRe.MatchString(strings.ToLower(strings.TrimSpace(raw)))
}

func NormalizeCategoryColor(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	if c == "" {
		return "", errors.New("укажите цвет #RRGGBB")
	}
	if !strings.HasPrefix(c, "#") {
		c = "#" + c
	}
	if len(c) == 4 { // #RGB → #RRGGBB
		c = fmt.Sprintf("#%c%c%c%c%c%c", c[1], c[1], c[2], c[2], c[3], c[3])
	}
	if !categoryColorRe.MatchString(c) {
		return "", errors.New("цвет: ожидается #RRGGBB")
	}
	return strings.ToLower(c), nil
}

// NormalizeDeviceCategory приводит значение к id типа; пустое → switch.
// Известные русские алиасы → builtins; произвольный валидный slug сохраняется (кастомный тип).
func NormalizeDeviceCategory(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return DeviceCategorySwitch
	}
	switch s {
	case DeviceCategorySwitch, "коммутатор", "коммутаторы":
		return DeviceCategorySwitch
	case DeviceCategoryRouter, "роутер", "роутеры", "маршрутизатор", "маршрутизаторы":
		return DeviceCategoryRouter
	case DeviceCategoryServer, "сервер", "серверы":
		return DeviceCategoryServer
	case DeviceCategoryComputer, "компьютер", "компьютеры", "pc":
		return DeviceCategoryComputer
	case DeviceCategoryPhone, "телефон", "телефоны", "voip":
		return DeviceCategoryPhone
	case DeviceCategoryMFU, "мфу", "принтер", "printer":
		return DeviceCategoryMFU
	case DeviceCategoryCamera, "камера", "камеры":
		return DeviceCategoryCamera
	case DeviceCategoryAP, "точка доступа", "точки доступа", "access point", "wifi", "wap":
		return DeviceCategoryAP
	case DeviceCategoryOther, "иное", "другие":
		return DeviceCategoryOther
	}
	if ValidCategoryID(s) {
		return s
	}
	return DeviceCategoryOther
}

// ValidDeviceCategory — sync-проверка формата id (builtins + кастомный slug).
func ValidDeviceCategory(raw string) bool {
	return ValidCategoryID(NormalizeDeviceCategory(raw))
}

func (s *Store) ListDeviceCategoryDefs(ctx context.Context) ([]DeviceCategoryDef, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, label, color, blink, builtin, sort_order, updated_at
		FROM device_category_defs
		ORDER BY sort_order ASC, label ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceCategoryDef
	for rows.Next() {
		var d DeviceCategoryDef
		if err := rows.Scan(&d.ID, &d.Label, &d.Color, &d.Blink, &d.Builtin, &d.SortOrder, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if out == nil {
		out = []DeviceCategoryDef{}
	}
	return out, rows.Err()
}

func (s *Store) DeviceCategoryExists(ctx context.Context, id string) (bool, error) {
	id = NormalizeDeviceCategory(id)
	var n int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM device_category_defs WHERE id = $1`, id).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CreateDeviceCategoryDef(ctx context.Context, id, label, color string, blink bool) (DeviceCategoryDef, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !ValidCategoryID(id) {
		return DeviceCategoryDef{}, errors.New("id: латиница, a-z, затем a-z0-9_, до 32 символов")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return DeviceCategoryDef{}, errors.New("укажите название типа")
	}
	if len([]rune(label)) > 64 {
		return DeviceCategoryDef{}, errors.New("название: максимум 64 символа")
	}
	col, err := NormalizeCategoryColor(color)
	if err != nil {
		return DeviceCategoryDef{}, err
	}
	var maxSort int
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(sort_order), 90) FROM device_category_defs`).Scan(&maxSort)
	sortOrder := maxSort + 10
	var d DeviceCategoryDef
	err = s.pool.QueryRow(ctx, `
		INSERT INTO device_category_defs (id, label, color, blink, builtin, sort_order, updated_at)
		VALUES ($1, $2, $3, $4, FALSE, $5, now())
		RETURNING id, label, color, blink, builtin, sort_order, updated_at`,
		id, label, col, blink, sortOrder,
	).Scan(&d.ID, &d.Label, &d.Color, &d.Blink, &d.Builtin, &d.SortOrder, &d.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return DeviceCategoryDef{}, fmt.Errorf("тип с id «%s» уже есть", id)
		}
		return DeviceCategoryDef{}, err
	}
	return d, nil
}

// UpdateDeviceCategoryDef: цвет и мигание — у любого; название — только у пользовательского.
func (s *Store) UpdateDeviceCategoryDef(ctx context.Context, id string, label *string, color *string, blink *bool) (DeviceCategoryDef, error) {
	id = NormalizeDeviceCategory(id)
	var cur DeviceCategoryDef
	err := s.pool.QueryRow(ctx, `
		SELECT id, label, color, blink, builtin, sort_order, updated_at
		FROM device_category_defs WHERE id = $1`, id,
	).Scan(&cur.ID, &cur.Label, &cur.Color, &cur.Blink, &cur.Builtin, &cur.SortOrder, &cur.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceCategoryDef{}, ErrCategoryNotFound
	}
	if err != nil {
		return DeviceCategoryDef{}, err
	}
	newLabel := cur.Label
	newColor := cur.Color
	newBlink := cur.Blink
	if label != nil {
		if cur.Builtin {
			return DeviceCategoryDef{}, errors.New("название встроенного типа менять нельзя")
		}
		t := strings.TrimSpace(*label)
		if t == "" {
			return DeviceCategoryDef{}, errors.New("укажите название типа")
		}
		if len([]rune(t)) > 64 {
			return DeviceCategoryDef{}, errors.New("название: максимум 64 символа")
		}
		newLabel = t
	}
	if color != nil {
		c, err := NormalizeCategoryColor(*color)
		if err != nil {
			return DeviceCategoryDef{}, err
		}
		newColor = c
	}
	if blink != nil {
		newBlink = *blink
	}
	err = s.pool.QueryRow(ctx, `
		UPDATE device_category_defs
		SET label = $2, color = $3, blink = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, label, color, blink, builtin, sort_order, updated_at`,
		id, newLabel, newColor, newBlink,
	).Scan(&cur.ID, &cur.Label, &cur.Color, &cur.Blink, &cur.Builtin, &cur.SortOrder, &cur.UpdatedAt)
	if err != nil {
		return DeviceCategoryDef{}, err
	}
	return cur, nil
}

// DeleteDeviceCategoryDef удаляет пользовательский тип; узлы переводит в other.
func (s *Store) DeleteDeviceCategoryDef(ctx context.Context, id string) error {
	id = NormalizeDeviceCategory(id)
	if id == DeviceCategoryOther {
		return errors.New("тип «other» удалять нельзя")
	}
	var builtin bool
	err := s.pool.QueryRow(ctx, `SELECT builtin FROM device_category_defs WHERE id = $1`, id).Scan(&builtin)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCategoryNotFound
	}
	if err != nil {
		return err
	}
	if builtin {
		return errors.New("встроенный тип удалять нельзя")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE devices SET device_category = $2, updated_at = now()
		WHERE device_category = $1`, id, DeviceCategoryOther); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM device_category_defs WHERE id = $1 AND builtin = FALSE`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCategoryNotFound
	}
	return tx.Commit(ctx)
}

// ErrCategoryNotFound — тип узла не найден в справочнике.
var ErrCategoryNotFound = errors.New("category not found")

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
