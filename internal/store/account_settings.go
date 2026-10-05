package store

import (
	"database/sql"
	"errors"
	"strings"
)

const (
	SettingUnarchiveChats = "unarchive_chats"
)

func (d *DB) GetAccountSetting(key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, errors.New("account setting key is required")
	}
	var val string
	err := d.sql.QueryRowContext(storeCtx(), `SELECT value FROM account_settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return val, true, nil
}

func (d *DB) SetAccountSetting(key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("account setting key is required")
	}
	_, err := d.sql.ExecContext(storeCtx(), `
		INSERT INTO account_settings(key, value)
		VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (d *DB) SetUnarchiveChatsSetting(permitted bool) error {
	val := "0"
	if permitted {
		val = "1"
	}
	return d.SetAccountSetting(SettingUnarchiveChats, val)
}

func (d *DB) UnarchiveChatsSettingPermitted() (bool, error) {
	val, ok, err := d.GetAccountSetting(SettingUnarchiveChats)
	if err != nil {
		return false, err
	}
	if !ok {
		// WhatsApp default: Keep chats archived is OFF, so auto-unarchive on incoming messages is permitted.
		return true, nil
	}
	return val == "1" || strings.EqualFold(val, "true"), nil
}

func (d *DB) UnarchiveChatIfPermitted(jid string) error {
	jid = strings.TrimSpace(jid)
	if jid == "" {
		return nil
	}
	permitted, err := d.UnarchiveChatsSettingPermitted()
	if err != nil {
		return err
	}
	if !permitted {
		return nil
	}
	_, err = d.sql.ExecContext(storeCtx(), `UPDATE chats SET archived = 0 WHERE jid = ? AND archived = 1`, jid)
	return err
}
