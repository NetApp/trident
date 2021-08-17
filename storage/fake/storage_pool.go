package fake

import (
	"encoding/json"

	sa "github.com/netapp/trident/v21/storage_attribute"
)

type StoragePool struct {
	Attrs map[string]sa.Offer `json:"attributes"`
	Bytes uint64              `json:"sizeBytes"`
}

// UnmarshalJSON implements json.Unmarshaler and allows FakeStoragePool
// to be unmarshaled with the Attrs map correctly defined.
func (p *StoragePool) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Attrs json.RawMessage `json:"attributes"`
		Bytes uint64          `json:"sizeBytes"`
	}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	p.Attrs, err = sa.UnmarshalOfferMap(tmp.Attrs)
	if err != nil {
		return err
	}
	p.Bytes = tmp.Bytes
	return nil
}

func (p *StoragePool) ConstructClone() *StoragePool {
	return &StoragePool{
		Attrs: p.Attrs,
		Bytes: p.Bytes,
	}
}
