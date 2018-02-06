// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func UnmarshalRequestMap(mapJSON json.RawMessage) (
	map[string]Request, error,
) {
	var tmp map[string]string
	ret := make(map[string]Request)

	if mapJSON == nil {
		return nil, nil
	}
	err := json.Unmarshal(mapJSON, &tmp)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal map: %v", err)
	}
	for name, stringVal := range tmp {
		ret[name], err = CreateAttributeRequestFromAttributeValue(name, stringVal)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func MarshalRequestMap(requestMap map[string]Request) ([]byte, error) {
	if requestMap == nil {
		return nil, nil
	}
	genericMap := make(map[string]string, len(requestMap))
	for k, v := range requestMap {
		genericMap[k] = v.String()
	}
	return json.Marshal(genericMap)
}

func CreateAttributeRequestFromAttributeValue(name, val string) (Request, error) {
	var req Request
	valType, ok := attrTypes[name]
	if !ok {
		return nil, fmt.Errorf("unrecognized storage attribute: %s", name)
	}
	switch valType {
	case boolType:
		v, err := strconv.ParseBool(val)
		if err != nil {
			return nil, fmt.Errorf("storage attribute value (%s) doesn't match the specified type (%s)", val, valType)
		}
		req = NewBoolRequest(v)
	case intType:
		v, err := strconv.ParseInt(val, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("storage attribute value (%s) doesn't match the specified type (%s)", val, valType)
		}
		req = NewIntRequest(int(v))
	case stringType:
		req = NewStringRequest(val)
	default:
		return nil, fmt.Errorf("unrecognized type for a storage attribute request: %s", valType)
	}
	return req, nil
}

func CreateBackendStoragePoolsMapFromEncodedString(
	arg string,
) (map[string][]string, error) {
	backendPoolsMap := make(map[string][]string)
	backendPoolsList := strings.Split(arg, ";")
	for _, backendPools := range backendPoolsList {
		vals := strings.SplitN(backendPools, ":", 2)
		if len(vals) != 2 || vals[0] == "" || vals[1] == "" {
			return nil, fmt.Errorf("the encoded backend-storage pool string does not have the right format")
		}
		backend := vals[0]
		Pools := strings.Split(vals[1], ",")
		backendPoolsMap[backend] = Pools
	}
	return backendPoolsMap, nil
}
