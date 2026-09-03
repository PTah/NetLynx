package traprecv

// TrapLabelOption — тип trap для UI-фильтра.
type TrapLabelOption struct {
	Value string `json:"value"`
	Title string `json:"title"`
	Group string `json:"group"`
}

// TrapLabelOptions — известные trap_label для чеклиста в настройках.
func TrapLabelOptions() []TrapLabelOption {
	return []TrapLabelOption{
		{Value: "linkUp", Title: "Порт подключён (linkUp)", Group: "Стандарт SNMP"},
		{Value: "linkDown", Title: "Порт отключён (linkDown)", Group: "Стандарт SNMP"},
		{Value: "coldStart", Title: "Холодный старт SNMP", Group: "Стандарт SNMP"},
		{Value: "warmStart", Title: "Тёплый старт SNMP", Group: "Стандарт SNMP"},
		{Value: "authenticationFailure", Title: "Ошибка SNMP auth", Group: "Стандарт SNMP"},
		{Value: "loginSessionStartStopTrap", Title: "CLI-сессия start/stop", Group: "EdgeSwitch"},
		{Value: "failedUserLoginTrap", Title: "Неудачный вход", Group: "EdgeSwitch"},
		{Value: "userLockoutTrap", Title: "Блокировка пользователя", Group: "EdgeSwitch"},
		{Value: "multipleUsersTrap", Title: "Несколько admin CLI", Group: "EdgeSwitch"},
		{Value: "loopDetectedTrap", Title: "STP: петля", Group: "EdgeSwitch"},
		{Value: "topologyChangeInitiatedTrap", Title: "Topology change", Group: "EdgeSwitch"},
		{Value: "agentSwitchStormControlTrap", Title: "Storm control", Group: "EdgeSwitch"},
		{Value: "agentSwitchCpuRisingThresholdTrap", Title: "CPU выше порога", Group: "EdgeSwitch"},
		{Value: "agentSwitchCpuFallingThresholdTrap", Title: "CPU ниже порога", Group: "EdgeSwitch"},
		{Value: "agentSwitchIpAddressConflictTrap", Title: "Конфликт IP", Group: "EdgeSwitch"},
		{Value: "dhcpSnoopingIntfErrorDisabledTrap", Title: "DHCP snoop: port disabled", Group: "EdgeSwitch"},
		{Value: "enterprise trap", Title: "Прочий vendor trap", Group: "Прочее"},
		{Value: "SNMP trap", Title: "Неизвестный / без OID", Group: "Прочее"},
	}
}

// RecommendedPortTrapLabels — пресет «интересные порты/линк».
func RecommendedPortTrapLabels() []string {
	return []string{"linkUp", "linkDown", "topologyChangeInitiatedTrap", "loopDetectedTrap", "agentSwitchStormControlTrap"}
}
