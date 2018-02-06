// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"fmt"
	"strings"
)

func NewStringOffer(offers ...string) Offer {
	return &stringOffer{
		Offers: offers,
	}
}

func (o *stringOffer) Matches(r Request) bool {
	sr, ok := r.(*stringRequest)
	if !ok {
		return false
	}
	for _, s := range o.Offers {
		if s == sr.Request {
			return true
		}
	}
	return false
}

func (o *stringOffer) String() string {
	return fmt.Sprintf("{Offers: %s}", strings.Join(o.Offers, ","))
}

func NewStringRequest(request string) Request {
	return &stringRequest{
		Request: request,
	}
}

func (r *stringRequest) Value() interface{} {
	return r.Request
}

func (r *stringRequest) GetType() Type {
	return stringType
}

func (r *stringRequest) String() string {
	return r.Request
}
