package controller

import (
	"github.com/netapp/trident/frontend/autogrow/cache"
)

// Controller is a lightweight layer that handles CRD events for autogrow
// It publishes SC, TVP, and TBE events to the ControllerEventBus for scheduler consumption
type Controller struct {
	// Cache for autogrow data
	cache *cache.AutogrowCache
}
