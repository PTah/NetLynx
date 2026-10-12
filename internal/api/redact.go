package api

import (
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// redactDeviceForAPI убирает SNMP community из JSON; выставляет has_community.
func redactDeviceForAPI(d *models.Device) {
	if d == nil {
		return
	}
	if d.Community != nil && strings.TrimSpace(*d.Community) != "" {
		d.HasCommunity = true
	}
	if d.SSHPassword != nil && strings.TrimSpace(*d.SSHPassword) != "" {
		d.HasSSHPassword = true
	}
	d.SSHPassword = nil
	if d.SSHEnablePassword != nil && strings.TrimSpace(*d.SSHEnablePassword) != "" {
		d.HasSSHEnablePassword = true
	}
	d.SSHEnablePassword = nil
	if strings.TrimSpace(d.SSHVendor) == "" {
		d.SSHVendor = "auto"
	}
}

func redactDevicesForAPI(list []models.Device) {
	for i := range list {
		redactDeviceForAPI(&list[i])
	}
}

// publicNotificationSettings — ответ GET без smtp_password / telegram_bot_token.
type publicNotificationSettings struct {
	WebhookURL                     *string `json:"webhook_url"`
	WebhookEnabled                 bool    `json:"webhook_enabled"`
	WebhookEventTypes              *string `json:"webhook_event_types"`
	WebhookSeverities              *string `json:"webhook_severities"`
	EmailEnabled                   bool    `json:"email_enabled"`
	EmailFrom                      *string `json:"email_from"`
	EmailTo                        *string `json:"email_to"`
	EmailEventTypes                *string `json:"email_event_types"`
	EmailSeverities                *string `json:"email_severities"`
	SMTPHost                       *string `json:"smtp_host"`
	SMTPPort                       int     `json:"smtp_port"`
	SMTPUsername                   *string `json:"smtp_username"`
	HasSMTPPassword                bool    `json:"has_smtp_password"`
	SMTPTLSSkipVerify              bool    `json:"smtp_tls_skip_verify"`
	HasTelegramBotToken            bool    `json:"has_telegram_bot_token"`
	TelegramChatID                 *string `json:"telegram_chat_id"`
	TelegramEnabled                bool    `json:"telegram_enabled"`
	TelegramEventTypes             *string `json:"telegram_event_types"`
	TelegramSeverities             *string `json:"telegram_severities"`
	NotifyMaxRetries               int     `json:"notify_max_retries"`
	NotifyRetryBackoffMs           int     `json:"notify_retry_backoff_ms"`
	IncidentActionEnabled          bool    `json:"incident_action_enabled"`
	IncidentActionEventTypes       *string `json:"incident_action_event_types"`
	IncidentActionDryRun           bool    `json:"incident_action_dry_run"`
	IncidentActionCooldownSeconds int     `json:"incident_action_cooldown_seconds"`
}

func toPublicNotifications(ns store.NotificationSettings) publicNotificationSettings {
	return publicNotificationSettings{
		WebhookURL:                     ns.WebhookURL,
		WebhookEnabled:                 ns.WebhookEnabled,
		WebhookEventTypes:              ns.WebhookEventTypes,
		WebhookSeverities:              ns.WebhookSeverities,
		EmailEnabled:                   ns.EmailEnabled,
		EmailFrom:                      ns.EmailFrom,
		EmailTo:                        ns.EmailTo,
		EmailEventTypes:                ns.EmailEventTypes,
		EmailSeverities:                ns.EmailSeverities,
		SMTPHost:                       ns.SMTPHost,
		SMTPPort:                       ns.SMTPPort,
		SMTPUsername:                   ns.SMTPUsername,
		HasSMTPPassword:                ns.SMTPPassword != nil && strings.TrimSpace(*ns.SMTPPassword) != "",
		SMTPTLSSkipVerify:              ns.SMTPTLSSkipVerify,
		HasTelegramBotToken:            ns.TelegramBotToken != nil && strings.TrimSpace(*ns.TelegramBotToken) != "",
		TelegramChatID:                 ns.TelegramChatID,
		TelegramEnabled:                ns.TelegramEnabled,
		TelegramEventTypes:             ns.TelegramEventTypes,
		TelegramSeverities:             ns.TelegramSeverities,
		NotifyMaxRetries:               ns.NotifyMaxRetries,
		NotifyRetryBackoffMs:           ns.NotifyRetryBackoffMs,
		IncidentActionEnabled:          ns.IncidentActionEnabled,
		IncidentActionEventTypes:       ns.IncidentActionEventTypes,
		IncidentActionDryRun:           ns.IncidentActionDryRun,
		IncidentActionCooldownSeconds:   ns.IncidentActionCooldownSeconds,
	}
}
