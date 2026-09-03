package store

import (
	"context"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

// CLIPortModeUpdate — обновление роли порта из show running-config.
type CLIPortModeUpdate struct {
	IfIndex     int
	CLIPortMode string
	PortRole    string
	AccessVLAN  *int
}

// CLIDescrUpdate — description из show running-config.
type CLIDescrUpdate struct {
	IfIndex     int
	Description string // может быть "" при no description
}

// ConfigPortApplyResult — итог ApplyConfigPortRoles (роли + описания).
type ConfigPortApplyResult struct {
	Roles         int
	Descriptions  int
}

func (r ConfigPortApplyResult) Total() int { return r.Roles + r.Descriptions }

// ListInterfaceNameIndex — if_index → if_name для сопоставления с конфигом.
func (s *Store) ListInterfaceNameIndex(ctx context.Context, deviceID int64) (map[int]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT if_index, COALESCE(if_name, '')
		FROM device_interfaces
		WHERE device_id = $1 AND if_index > 0
		ORDER BY if_index`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]string)
	for rows.Next() {
		var idx int
		var name string
		if err := rows.Scan(&idx, &name); err != nil {
			return nil, err
		}
		out[idx] = name
	}
	return out, rows.Err()
}

// UpdateInterfaceCLIPortModes пишет cli_port_mode и port_role из конфига.
func (s *Store) UpdateInterfaceCLIPortModes(ctx context.Context, deviceID int64, updates []CLIPortModeUpdate, syncedAt time.Time) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	n := 0
	for _, u := range updates {
		if u.CLIPortMode == "" || u.PortRole == "" {
			continue
		}
		tag, err := s.pool.Exec(ctx, `
			UPDATE device_interfaces SET
				cli_port_mode = $3,
				cli_access_vlan = $4,
				cli_mode_synced_at = $5,
				port_role = $6,
				updated_at = now()
			WHERE device_id = $1 AND if_index = $2`,
			deviceID, u.IfIndex, u.CLIPortMode, u.AccessVLAN, syncedAt, u.PortRole)
		if err != nil {
			return n, err
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

// UpdateInterfaceCLIDescriptions пишет cli_description и if_descr (если нет descr_override).
func (s *Store) UpdateInterfaceCLIDescriptions(ctx context.Context, deviceID int64, updates []CLIDescrUpdate) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	n := 0
	for _, u := range updates {
		var descrArg interface{}
		if strings.TrimSpace(u.Description) == "" {
			descrArg = nil
		} else {
			descrArg = strings.TrimSpace(u.Description)
		}
		tag, err := s.pool.Exec(ctx, `
			UPDATE device_interfaces SET
				cli_description = $3,
				if_descr = CASE
					WHEN descr_override IS NOT NULL AND btrim(descr_override) <> '' THEN if_descr
					ELSE $3
				END,
				updated_at = now()
			WHERE device_id = $1 AND if_index = $2`,
			deviceID, u.IfIndex, descrArg)
		if err != nil {
			return n, err
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

// ApplyConfigPortRoles разбирает running-config: роли (switchport) и description портов.
func (s *Store) ApplyConfigPortRoles(ctx context.Context, deviceID int64, configRaw []byte) (ConfigPortApplyResult, error) {
	var res ConfigPortApplyResult
	if len(configRaw) == 0 {
		return res, nil
	}
	parsed := swcfg.ParseRunningConfigPortModes(string(configRaw))
	if len(parsed) == 0 {
		return res, nil
	}
	ifNames, err := s.ListInterfaceNameIndex(ctx, deviceID)
	if err != nil {
		return res, err
	}
	if len(ifNames) == 0 {
		return res, nil
	}
	now := time.Now()
	var roleUpdates []CLIPortModeUpdate
	var descrUpdates []CLIDescrUpdate
	seenRole := make(map[int]struct{})
	seenDescr := make(map[int]struct{})
	for _, p := range parsed {
		ifIndex, ok := swcfg.MatchConfigIfaceToIfIndex(p.IfaceName, ifNames)
		if !ok {
			continue
		}
		if strings.TrimSpace(p.Mode) != "" {
			role := swcfg.PortRoleFromCLIMode(p.Mode)
			if role != "" {
				if _, dup := seenRole[ifIndex]; !dup {
					seenRole[ifIndex] = struct{}{}
					access := p.AccessVLAN
					if access == nil {
						access = p.PVID
					}
					roleUpdates = append(roleUpdates, CLIPortModeUpdate{
						IfIndex:     ifIndex,
						CLIPortMode: strings.ToLower(strings.TrimSpace(p.Mode)),
						PortRole:    role,
						AccessVLAN:  access,
					})
				}
			}
		}
		if p.HasDescr {
			if _, dup := seenDescr[ifIndex]; !dup {
				seenDescr[ifIndex] = struct{}{}
				descrUpdates = append(descrUpdates, CLIDescrUpdate{
					IfIndex:     ifIndex,
					Description: p.Description,
				})
			}
		}
	}
	res.Roles, err = s.UpdateInterfaceCLIPortModes(ctx, deviceID, roleUpdates, now)
	if err != nil {
		return res, err
	}
	res.Descriptions, err = s.UpdateInterfaceCLIDescriptions(ctx, deviceID, descrUpdates)
	return res, err
}

// GetDeviceCLIModeSyncAt — время последней синхронизации switchport mode из show run (max по портам).
func (s *Store) GetDeviceCLIModeSyncAt(ctx context.Context, deviceID int64) (*time.Time, error) {
	if deviceID <= 0 {
		return nil, nil
	}
	var t *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT max(cli_mode_synced_at) FROM device_interfaces WHERE device_id = $1`, deviceID,
	).Scan(&t)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateInterfaceVLANAfterCLI обновляет роль/access VLAN после SSH-команды на порту.
func (s *Store) UpdateInterfaceVLANAfterCLI(ctx context.Context, deviceID int64, ifIndex int, op string, vlanID int) error {
	now := time.Now()
	switch op {
	case swcfg.VLANOpSetAccess:
		_, err := s.pool.Exec(ctx, `
			UPDATE device_interfaces SET
				cli_port_mode = 'access',
				port_role = 'access',
				cli_access_vlan = $3,
				cli_mode_synced_at = $4,
				updated_at = now()
			WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, vlanID, now)
		return err
	case swcfg.VLANOpAddTagged, swcfg.VLANOpTrunkAllow:
		_, err := s.pool.Exec(ctx, `
			UPDATE device_interfaces SET
				cli_port_mode = 'trunk',
				port_role = 'trunk',
				cli_mode_synced_at = $3,
				updated_at = now()
			WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, now)
		return err
	case swcfg.VLANOpRemove:
		_, err := s.pool.Exec(ctx, `
			UPDATE device_interfaces SET
				cli_access_vlan = CASE WHEN cli_access_vlan = $3 THEN 1 ELSE cli_access_vlan END,
				cli_mode_synced_at = $4,
				updated_at = now()
			WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, vlanID, now)
		return err
	default:
		return nil
	}
}

// ResolveInterfacePortRole — роль порта для топологии/FDB: cli_port_mode из show run главнее port_role.
func ResolveInterfacePortRole(portRole string, cliPortMode *string) string {
	if cliPortMode != nil {
		switch strings.ToLower(strings.TrimSpace(*cliPortMode)) {
		case "trunk":
			return "trunk"
		case "access":
			return "access"
		}
	}
	switch strings.ToLower(strings.TrimSpace(portRole)) {
	case "trunk", "ignore", "access":
		return strings.ToLower(strings.TrimSpace(portRole))
	}
	return "access"
}

// PortRolesForFDBTopology — роли для FDB→топология: только cli_port_mode / port_role из конфига.
func PortRolesForFDBTopology(ifs map[int]InterfaceSnapshot) map[int]string {
	out := make(map[int]string, len(ifs))
	for ifIndex, snap := range ifs {
		out[ifIndex] = ResolveInterfacePortRole(snap.PortRole, snap.CLIPortMode)
	}
	return out
}
