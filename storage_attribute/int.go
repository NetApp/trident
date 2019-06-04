// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"fmt"
)

func NewIntOffer(min, max int) Offer {
	return &intOffer{
		Min: min,
		Max: max,
	}
}

func (o *intOffer) Matches(r Request) bool {
	ir, ok := r.(*intRequest)
	if !ok {
		return false
	}
	return ir.Request >= o.Min && ir.Request <= o.Max
}

func (o *intOffer) String() string {
	return fmt.Sprintf("{Min: %d, Max: %d}", o.Min, o.Max)
}

func (o *intOffer) ToString() string {
	return o.String()
}

func NewIntRequest(request int) Request {
	return &intRequest{
		Request: request,
	}
}

func (r *intRequest) Value() interface{} {
	return r.Request
}

func (r *intRequest) GetType() Type {
	return intType
}

func (r *intRequest) String() string {
	return fmt.Sprintf("%d", r.Request)
}
