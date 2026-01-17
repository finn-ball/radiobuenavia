package dropbox

import (
	"encoding/json"
	"fmt"
)

type Account struct {
	AccountID string
	Email     string
	Name      string
}

func (c *Client) GetCurrentAccount() (Account, error) {
	resp, err := c.doAPIRequestNoBody("/2/users/get_current_account")
	if err != nil {
		return Account{}, err
	}
	var payload struct {
		AccountID string `json:"account_id"`
		Email     string `json:"email"`
		Name      struct {
			DisplayName string `json:"display_name"`
		} `json:"name"`
	}
	if err := json.Unmarshal(resp, &payload); err != nil {
		return Account{}, err
	}
	if payload.AccountID == "" {
		return Account{}, fmt.Errorf("dropbox account response missing account_id")
	}
	return Account{
		AccountID: payload.AccountID,
		Email:     payload.Email,
		Name:      payload.Name.DisplayName,
	}, nil
}
