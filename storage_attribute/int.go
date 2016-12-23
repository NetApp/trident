// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_attribute

import (
	"fmt"
)

func NewIntOffer(min, max int) Offer {
	return &intOffer{
		Min: min,
		Max: max,
	}
}

func (a *intOffer) Matches(r Request) bool {
	ir, ok := r.(*intRequest)
	if !ok {
		return false
	}
	return ir.Request >= a.Min && ir.Request <= a.Max
}

func (o *intOffer) String() string {
	return fmt.Sprintf("{Min: %d, Max: %d}", o.Min, o.Max)
}

func NewIntRequest(request int) Request {
	return &intRequest{
		Request: request,
	}
}

func (r *intRequest) Value() interface{} {
	return r.Request
}

func (r *intRequest) GetType() StorageAttributeType {
	return intType
}

func (r *intRequest) String() string {
	return fmt.Sprintf("%d", r.Request)
}
