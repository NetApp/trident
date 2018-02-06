// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"errors"

	log "github.com/sirupsen/logrus"
)

// AddAccount tbd
func (c *Client) AddAccount(req *AddAccountRequest) (accountID int64, err error) {
	var result AddAccountResult
	response, err := c.Request("AddAccount", req, NewReqID())
	if err != nil {
		log.Errorf("Error detected in AddAccount API response: %+v", err)
		return 0, errors.New("device API error")
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected in AddAccount API response: %+v", err)
		return 0, errors.New("device API error")
	}
	return result.Result.AccountID, nil
}

// GetAccountByName tbd
func (c *Client) GetAccountByName(req *GetAccountByNameRequest) (account Account, err error) {
	response, err := c.Request("GetAccountByName", req, NewReqID())
	if err != nil {
		return
	}

	var result GetAccountResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling GetAccountByName API response: %+v", err)
		return Account{}, errors.New("json-decode error")
	}
	log.Debugf("returning account: %+v", result.Result.Account)
	return result.Result.Account, err
}

// GetAccountByID tbd
func (c *Client) GetAccountByID(req *GetAccountByIDRequest) (account Account, err error) {
	var result GetAccountResult
	response, err := c.Request("GetAccountByID", req, NewReqID())
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Errorf("Error detected unmarshalling GetAccountByID API response: %+v", err)
		return account, errors.New("json-decode error")
	}
	return result.Result.Account, err
}
