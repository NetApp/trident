// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/netapp/trident/logging"
)

// AddAccount tbd
func (c *Client) AddAccount(ctx context.Context, req *AddAccountRequest) (accountID int64, err error) {
	var result AddAccountResult
	response, err := c.Request(ctx, "AddAccount", req, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error detected in AddAccount API response: %+v", err)
		return 0, errors.New("device API error")
	}

	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected in AddAccount API response: %+v", err)
		return 0, errors.New("device API error")
	}
	return result.Result.AccountID, nil
}

// GetAccountByName tbd
func (c *Client) GetAccountByName(ctx context.Context, req *GetAccountByNameRequest) (account Account, err error) {
	response, err := c.Request(ctx, "GetAccountByName", req, NewReqID())
	if err != nil {
		return
	}

	var result GetAccountResult
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling GetAccountByName API response: %+v", err)
		return Account{}, errors.New("json-decode error")
	}
	Logc(ctx).Debugf("returning account: %+v", result.Result.Account)
	return result.Result.Account, err
}

// GetAccountByID tbd
func (c *Client) GetAccountByID(ctx context.Context, req *GetAccountByIDRequest) (account Account, err error) {
	var result GetAccountResult
	response, err := c.Request(ctx, "GetAccountByID", req, NewReqID())
	if err := json.Unmarshal(response, &result); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling GetAccountByID API response: %+v", err)
		return account, errors.New("json-decode error")
	}
	return result.Result.Account, err
}
