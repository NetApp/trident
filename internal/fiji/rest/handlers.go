package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// makeListFaultsHandler lists ALL fault configuration that exist.
func makeListFaultsHandler(store FaultStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		var enabled *bool

		// Check if enabled param is present as a route variable. If it is, then use it.
		params := r.URL.Query()
		if params.Get("enabled") != "" {
			// Enabled represents 3 states: set / not set, true, and false.
			// Therefore, it should only be set if the query param exists.
			enabledSet, err := strconv.ParseBool(params.Get("enabled"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			enabled = &enabledSet
		}

		// Build a response body.
		faults := store.List()
		externalFaults := make([]any, 0, len(faults))
		for _, fault := range faults {
			// If the enabled query param isn't set, add every fault to the list.
			if enabled == nil {
				externalFaults = append(externalFaults, fault)
				continue
			}

			// Add the current fault to the list if its enabled state equals the query params value.
			if *enabled == fault.IsHandlerSet() {
				externalFaults = append(externalFaults, fault)
				continue
			}
		}

		// Get the faults stored in the store and convert them to JSON.
		data, err := json.Marshal(externalFaults)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	}
}

// makeGetFaultHandler retrieves a single fault configuration that exists.
func makeGetFaultHandler(store FaultStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")

		// Check if the name is present as a route variable. If it is, then use it.
		name, ok := mux.Vars(r)["name"]
		if !ok {
			http.Error(w, "no fault config specified", http.StatusBadRequest)
			return
		}

		fault, exists := store.Get(name)
		if !exists || fault == nil {
			http.Error(w, "no fault found", http.StatusNotFound)
			return
		}

		// Get the faults stored in the store and convert them to JSON.
		data, err := json.Marshal(fault)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	}
}

// makeSetFaultConfigHandler attempts to put a single fault configuration and handler for a registered fault.
func makeSetFaultConfigHandler(store FaultStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")

		name, ok := mux.Vars(r)["name"]
		if !ok {
			http.Error(w, "no fault name specified", http.StatusBadRequest)
			return
		}

		if exists := store.Exists(name); !exists {
			http.Error(w, fmt.Sprintf("specified fault [%s] does not exist", name), http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if len(body) == 0 {
			http.Error(w, "empty request body", http.StatusBadRequest)
			return
		}

		if err = store.Set(name, body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		return
	}
}

// makeResetFaultConfigHandler attempts to reset a single faults handler config to its original state.
func makeResetFaultConfigHandler(store FaultStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")

		name, ok := mux.Vars(r)["name"]
		if !ok || name == "" {
			http.Error(w, "no fault name specified", http.StatusBadRequest)
			return
		}

		if exists := store.Exists(name); !exists {
			http.Error(w, fmt.Sprintf("specified fault [%s] does not exist", name), http.StatusNotFound)
			return
		}

		// Ignore any payload and just reset the fault.
		if err := store.Reset(name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		return
	}
}
