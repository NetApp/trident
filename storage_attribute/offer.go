// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_attribute

import (
	"encoding/json"
	"fmt"
)

func UnmarshalOfferMap(mapJSON json.RawMessage) (
	map[string]Offer, error,
) {
	var tmp map[string]json.RawMessage
	ret := make(map[string]Offer)

	err := json.Unmarshal(mapJSON, &tmp)
	if err != nil {
		return nil, fmt.Errorf("Unable to unmarshal map:  %v", err)
	}
	for name, rawAttr := range tmp {
		var (
			final Offer
		)
		baseType, ok := attrTypes[name]
		if !ok {
			return nil, fmt.Errorf("Unknown storage attribute:  %s", name)
		}
		switch {
		case baseType == boolType:
			final = new(boolOffer)
		case baseType == intType:
			final = new(intOffer)
		case baseType == stringType:
			final = new(stringOffer)
		default:
			return nil, fmt.Errorf("Offer %s has unrecognized type %s", name,
				baseType)
		}
		err = json.Unmarshal(rawAttr, final)
		if err != nil {
			return nil, fmt.Errorf("Unable to fully unmarshal request %s:  %v",
				name, err)
		}
		ret[name] = final
	}
	return ret, nil
}
