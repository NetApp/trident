// Package factory provides common validation and registration functionality for object store drivers.
package factory

import (
	"context"
	"fmt"

	"github.com/netapp/trident/object"
)

// StoreConstructor is a function that lays out an instance of an object store driver. It must not perform any initialization or attempt to connect to the storage.
// Each config document is unmarshaled onto the same driver in order, so keys in later documents win and keys absent
// from later documents are kept. Validation must happen only after every document has been applied.
type StoreConstructor func(ctx context.Context, secretConfig []byte, configs ...[]byte) (object.Store, error)

var drivers = make(map[string]StoreConstructor)

func RegisterStoreDriver(name string, driver StoreConstructor) {
	drivers[name] = driver
}

func CreateStore(ctx context.Context, driverName string, secretConfig []byte, configs ...[]byte) (object.Store, error) {
	ctor, ok := drivers[driverName]
	if !ok {
		return nil, fmt.Errorf("driver not found: %s", driverName)
	}
	return ctor(ctx, secretConfig, configs...)
}
