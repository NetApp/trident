// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

type Offer interface {
	Matches(requested Request) bool
}

// At the moment, there aren't any terribly useful methods to put here, but
// there might be.  This is more here for symmetry at the moment.
type Request interface {
	GetType() Type
	Value() interface{}
	String() string
}

type Type string

const (
	intType    Type = "int"
	boolType   Type = "bool"
	stringType Type = "string"
)

type intOffer struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type intRequest struct {
	Request int `json:"request"`
}

type boolOffer struct {
	Offer bool `json:"offer"`
}

type boolRequest struct {
	Request bool `json:"request"`
}

type stringOffer struct {
	Offers []string `json:"offer"`
}

type stringRequest struct {
	Request string `json:"request"`
}
