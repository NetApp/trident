// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_attribute

import (
	"fmt"
)

func NewBoolOffer(offer bool) Offer {
	return &boolOffer{
		Offer: offer,
	}
}

// A boolean offer of true matches any request; a boolean offer of false
// only matches a false request.  This assumes that the requested parameter
// will be passed into the driver.
func (a *boolOffer) Matches(r Request) bool {
	br, ok := r.(*boolRequest)
	if !ok {
		return false
	}
	if a.Offer {
		return true
	}
	return br.Request == a.Offer
}

func (o *boolOffer) String() string {
	return fmt.Sprintf("{Offer:  %t}", o.Offer)
}

func NewBoolRequest(request bool) Request {
	return &boolRequest{
		Request: request,
	}
}

func (r *boolRequest) Value() interface{} {
	return r.Request
}

func (r *boolRequest) GetType() StorageAttributeType {
	return boolType
}

func (r *boolRequest) String() string {
	return fmt.Sprintf("%t", r.Request)
}
