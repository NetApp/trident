package handlers

import "encoding/json"

type NeverErrorHandler struct {
	Name string `json:"name"`
}

func (n *NeverErrorHandler) Handle() error {
	return nil
}

func NewNeverErrorHandler(model []byte) (*NeverErrorHandler, error) {
	var handler NeverErrorHandler
	if err := json.Unmarshal(model, &handler); err != nil {
		return nil, err
	}

	return &handler, nil
}
