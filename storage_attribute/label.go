// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

var (
	labelEqualRegex     = regexp.MustCompile(`^(?P<labelName>[\w-]+)\s*={1,2}\s*(?P<labelValue>[\w-]+)$`)
	labelNotEqualRegex  = regexp.MustCompile(`^(?P<labelName>[\w-]+)\s*!=\s*(?P<labelValue>[\w-]+)$`)
	labelInSetRegex     = regexp.MustCompile(`^(?P<labelName>[\w-]+)\s+in\s+[(](?P<labelSet>[\s\w,-]+)[)]$`)
	labelNotInSetRegex  = regexp.MustCompile(`^(?P<labelName>[\w-]+)\s+notin\s+[(](?P<labelSet>[\s\w,-]+)[)]$`)
	labelExistsRegex    = regexp.MustCompile(`^(?P<labelName>[\w-]+)$`)
	labelNotExistsRegex = regexp.MustCompile(`^!(?P<labelName>[\w-]+)$`)
)

type LabelOffer interface {
	Offer
	Labels() map[string]string
}

func NewLabelOffer(labelMaps ...map[string]string) Offer {
	// Combine multiple maps into a single map
	offers := make(map[string]string)

	for _, labelMap := range labelMaps {
		for k, v := range labelMap {
			offers[k] = v
		}
	}

	log.WithField("offers", offers).Debug("NewLabelOffer")

	return &labelOffer{
		Offers: offers,
	}
}

func (o *labelOffer) Matches(r Request) bool {
	log.WithFields(log.Fields{
		"request": r,
		"offers":  o.Offers,
	}).Debug("Matches")

	// Check that this is a label request
	request, ok := r.(*labelRequest)
	if !ok {
		return false
	}

	// Check that each selector finds a match among the offered labels
	for _, selector := range request.selectors {
		if !selector.Matches(*o) {
			return false
		}
	}

	return true
}

func (o *labelOffer) String() string {
	return fmt.Sprintf("{Offers: %v}", o.Offers)
}

func (o *labelOffer) ToString() string {
	return fmt.Sprintf("%v", o.Offers)
}

func (o *labelOffer) Labels() map[string]string {
	mapCopy := make(map[string]string)
	for k, v := range o.Offers {
		mapCopy[k] = v
	}
	return mapCopy
}

func NewLabelRequest(request string) (Request, error) {
	log.WithField("request", request).Debug("NewLabelRequest")

	if len(request) == 0 {
		return nil, fmt.Errorf("label selector may not be empty")
	}

	// Split selector line into individual selectors and parse each according to its type
	var selectors []labelSelector
	for _, r := range strings.Split(request, ";") {

		r = strings.TrimSpace(r)

		if labelEqualRegex.MatchString(r) {
			selectors = append(selectors, newLabelEqualRequest(r))
		} else if labelNotEqualRegex.MatchString(r) {
			selectors = append(selectors, newLabelNotEqualRequest(r))
		} else if labelInSetRegex.MatchString(r) {
			selectors = append(selectors, newLabelInSetRequest(r))
		} else if labelNotInSetRegex.MatchString(r) {
			selectors = append(selectors, newLabelNotInSetRequest(r))
		} else if labelExistsRegex.MatchString(r) {
			selectors = append(selectors, newLabelExistsRequest(r))
		} else if labelNotExistsRegex.MatchString(r) {
			selectors = append(selectors, newLabelNotExistsRequest(r))
		} else {
			return nil, fmt.Errorf("invalid label selector: %s", r)
		}
	}

	return &labelRequest{
		Request:   request,
		selectors: selectors,
	}, nil
}

func NewLabelRequestMustCompile(request string) Request {
	r, err := NewLabelRequest(request)
	if err != nil {
		panic(err)
	}
	return r
}

func (r *labelRequest) Value() interface{} {
	return r.Request
}

func (r *labelRequest) GetType() Type {
	return labelType
}

func (r *labelRequest) String() string {
	return r.Request
}

// Common interface for the various types of label requests (==, !=, in, notin, exists)
type labelSelector interface {
	Matches(offer labelOffer) bool
}

/////////////////////////////////////////////////////////////////////////////
// labelSelector for equality (equals)
/////////////////////////////////////////////////////////////////////////////

type labelEqualRequest struct {
	labelName  string
	labelValue string
}

func newLabelEqualRequest(request string) labelSelector {
	match := labelEqualRegex.FindStringSubmatch(request)
	paramsMap := make(map[string]string)
	for i, name := range labelEqualRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	return &labelEqualRequest{
		labelName:  paramsMap["labelName"],
		labelValue: paramsMap["labelValue"],
	}
}

func (r *labelEqualRequest) Matches(offer labelOffer) bool {
	for labelName, labelValue := range offer.Offers {
		if r.labelName == labelName && r.labelValue == labelValue {
			return true
		}
	}
	return false
}

/////////////////////////////////////////////////////////////////////////////
// labelSelector for equality (not equals)
/////////////////////////////////////////////////////////////////////////////

type labelNotEqualRequest struct {
	labelName  string
	labelValue string
}

func newLabelNotEqualRequest(request string) labelSelector {
	match := labelNotEqualRegex.FindStringSubmatch(request)
	paramsMap := make(map[string]string)
	for i, name := range labelNotEqualRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	return &labelNotEqualRequest{
		labelName:  paramsMap["labelName"],
		labelValue: paramsMap["labelValue"],
	}
}

func (r *labelNotEqualRequest) Matches(offer labelOffer) bool {
	for labelName, labelValue := range offer.Offers {
		if r.labelName == labelName && r.labelValue != labelValue {
			return true
		}
	}
	return false
}

/////////////////////////////////////////////////////////////////////////////
// labelSelector for sets (in)
/////////////////////////////////////////////////////////////////////////////

type labelInSetRequest struct {
	labelName string
	labelSet  []string
}

func newLabelInSetRequest(request string) labelSelector {
	match := labelInSetRegex.FindStringSubmatch(request)
	paramsMap := make(map[string]string)
	for i, name := range labelInSetRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	labelSet := make([]string, 0)
	for _, value := range strings.Split(paramsMap["labelSet"], ",") {
		value := strings.TrimSpace(value)
		if value != "" {
			labelSet = append(labelSet, value)
		}
	}

	return &labelInSetRequest{
		labelName: paramsMap["labelName"],
		labelSet:  labelSet,
	}
}

func (r *labelInSetRequest) Matches(offer labelOffer) bool {
	for labelName, labelValue := range offer.Offers {
		if r.labelName == labelName {
			// Found match in key
			for _, setValue := range r.labelSet {
				if setValue == labelValue {
					// Found match in values set
					return true
				}
			}
		}
	}
	return false
}

/////////////////////////////////////////////////////////////////////////////
// labelSelector for sets (notin)
/////////////////////////////////////////////////////////////////////////////

type labelNotInSetRequest struct {
	labelName string
	labelSet  []string
}

func newLabelNotInSetRequest(request string) labelSelector {
	match := labelNotInSetRegex.FindStringSubmatch(request)
	paramsMap := make(map[string]string)
	for i, name := range labelNotInSetRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	labelSet := make([]string, 0)
	for _, value := range strings.Split(paramsMap["labelSet"], ",") {
		value := strings.TrimSpace(value)
		if value != "" {
			labelSet = append(labelSet, value)
		}
	}

	return &labelNotInSetRequest{
		labelName: paramsMap["labelName"],
		labelSet:  labelSet,
	}
}

func (r *labelNotInSetRequest) Matches(offer labelOffer) bool {
	for labelName, labelValue := range offer.Offers {
		if r.labelName == labelName {
			// Found match in key
			for _, setValue := range r.labelSet {
				if setValue == labelValue {
					// Found match in set --> no match
					return false
				}
			}
			// Found key but no match in set --> match
			return true
		}
	}
	// Found no match in key --> match
	return true
}

/////////////////////////////////////////////////////////////////////////////
// labelSelector for sets (exists)
/////////////////////////////////////////////////////////////////////////////

type labelExistsRequest struct {
	labelName string
}

func newLabelExistsRequest(request string) labelSelector {
	match := labelExistsRegex.FindStringSubmatch(request)
	paramsMap := make(map[string]string)
	for i, name := range labelExistsRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	return &labelExistsRequest{
		labelName: paramsMap["labelName"],
	}
}

func (r *labelExistsRequest) Matches(offer labelOffer) bool {
	for labelName := range offer.Offers {
		if r.labelName == labelName {
			// Found match in key --> match
			return true
		}
	}
	// Found no match in key --> no match
	return false
}

/////////////////////////////////////////////////////////////////////////////
// labelSelector for sets (not exists)
/////////////////////////////////////////////////////////////////////////////

type labelNotExistsRequest struct {
	labelName string
}

func newLabelNotExistsRequest(request string) labelSelector {
	match := labelNotExistsRegex.FindStringSubmatch(request)
	paramsMap := make(map[string]string)
	for i, name := range labelNotExistsRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	return &labelNotExistsRequest{
		labelName: paramsMap["labelName"],
	}
}

func (r *labelNotExistsRequest) Matches(offer labelOffer) bool {
	for labelName := range offer.Offers {
		if r.labelName == labelName {
			// Found match in key --> no match
			return false
		}
	}
	// Found no match in key --> match
	return true
}
