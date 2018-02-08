// Copyright 2016 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"

	log "github.com/Sirupsen/logrus"
)

type ZAPIRequest interface {
	ToXML() (string, error)
}

type ZapiRunner struct {
	ManagementLIF   string
	SVM             string
	Username        string
	Password        string
	Secure          bool
	OntapiVersion   string
	DebugTraceFlags map[string]bool // Example: {"api":false, "method":true}
}

// SendZapi sends the provided ZAPIRequest to the Ontap system
func (o *ZapiRunner) SendZapi(r ZAPIRequest) (*http.Response, error) {

	if o.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "SendZapi", "Type": "ZapiRunner"}
		log.WithFields(fields).Debug(">>>> SendZapi")
		defer log.WithFields(fields).Debug("<<<< SendZapi")
	}

	zapiCommand, err := r.ToXML()
	if err != nil {
		return nil, err
	}

	var s = ""
	if o.SVM == "" {
		s = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
        <netapp xmlns="http://www.netapp.com/filer/admin" version="1.21">
            %s
        </netapp>`, zapiCommand)
	} else {
		s = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
        <netapp xmlns="http://www.netapp.com/filer/admin" version="1.21" %s>
            %s
        </netapp>`, "vfiler=\""+o.SVM+"\"", zapiCommand)
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("sending to '%s' xml: \n%s", o.ManagementLIF, s)
	}

	url := "http://" + o.ManagementLIF + "/servlets/netapp.servlets.admin.XMLrequest_filer"
	if o.Secure {
		url = "https://" + o.ManagementLIF + "/servlets/netapp.servlets.admin.XMLrequest_filer"
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("URL:> %s", url)
	}

	b := []byte(s)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(o.Username, o.Password)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if o.DebugTraceFlags["api"] {
		log.Debugf("response Status: %s", resp.Status)
		log.Debugf("response Headers: %s", resp.Header)
	}

	return resp, err
}
