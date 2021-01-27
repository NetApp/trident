// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"time"

	tridentconfig "github.com/netapp/trident/config"
	log "github.com/sirupsen/logrus"
)

type ZAPIRequest interface {
	ToXML() (string, error)
}

type ZAPIResponseIterable interface {
	NextTag() string
}

type ZapiRunner struct {
	ManagementLIF        string
	SVM                  string
	Username             string
	Password             string
	ClientPrivateKey     string
	ClientCertificate    string
	TrustedCACertificate string
	Secure               bool
	OntapiVersion        string
	DebugTraceFlags      map[string]bool // Example: {"api":false, "method":true}
}

// GetZAPIName returns the name of the ZAPI request; it must parse the XML because ZAPIRequest is an interface
//   See also: https://play.golang.org/p/IqHhgVB3Q7x
func GetZAPIName(zr ZAPIRequest) (string, error) {
	zapiXML, err := zr.ToXML()
	if err != nil {
		return "", err
	}

	decoder := xml.NewDecoder(strings.NewReader(zapiXML))
	for {
		token, _ := decoder.Token()
		if token == nil {
			break
		}
		switch startElement := token.(type) {
		case xml.StartElement:
			return startElement.Name.Local, nil
		}
	}
	return "", fmt.Errorf("could not find start tag for ZAPI: %v", zapiXML)
}

// SendZapi sends the provided ZAPIRequest to the Ontap system
func (o *ZapiRunner) SendZapi(r ZAPIRequest) (*http.Response, error) {

	startTime := time.Now()

	if o.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "SendZapi", "Type": "ZapiRunner"}
		log.WithFields(fields).Debug(">>>> SendZapi")
		defer log.WithFields(fields).Debug("<<<< SendZapi")
	}

	zapiCommand, err := r.ToXML()
	if err != nil {
		return nil, err
	}

	zapiName, zapiNameErr := GetZAPIName(r)
	if zapiNameErr == nil {
		zapiOpsTotal.WithLabelValues(o.SVM, zapiName).Inc()
		defer func() {
			endTime := float64(time.Since(startTime).Milliseconds())
			zapiOpsDurationInMsBySVMSummary.WithLabelValues(o.SVM, zapiName).Observe(endTime)
		}()
	}

	s := ""
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
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml")

	// Check to use cert/key and load the cert pair
	var cert tls.Certificate
	caCertPool := x509.NewCertPool()
	skipVerify := true
	if o.ClientCertificate != "" && o.ClientPrivateKey != "" {
		certDecode, err := base64.StdEncoding.DecodeString(o.ClientCertificate)
		if err != nil {
			return nil, errors.New("failed to decode client certificate from base64")
		}
		keyDecode, err := base64.StdEncoding.DecodeString(o.ClientPrivateKey)
		if err != nil {
			return nil, errors.New("failed to decode private key from base64")
		}
		cert, err = tls.X509KeyPair(certDecode, keyDecode)
		if err != nil {
			log.Debugf("error: %v", err)
			return nil, errors.New("cannot load certificate and key")
		}
	} else {
		req.SetBasicAuth(o.Username, o.Password)
	}

	// Check to use trustedCACertificate to use InsecureSkipVerify or not
	if o.TrustedCACertificate != "" {
		trustedCACert, err := base64.StdEncoding.DecodeString(o.TrustedCACertificate)
		if err != nil {
			return nil, errors.New("failed to decode trusted CA certificate from base64")
		}
		skipVerify = false
		caCertPool.AppendCertsFromPEM(trustedCACert)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerify, MinVersion: tridentconfig.MinTLSVersion,
			Certificates: []tls.Certificate{cert}, RootCAs: caCertPool},
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(tridentconfig.StorageAPITimeoutSeconds * time.Second),
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	} else if response.StatusCode == 401 {
		return nil, errors.New("response code 401 (Unauthorized): incorrect or missing credentials")
	}

	if o.DebugTraceFlags["api"] {
		log.Debugf("response Status: %s", response.Status)
		log.Debugf("response Headers: %s", response.Header)
	}

	return response, err
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ZapiRunner) ExecuteUsing(z ZAPIRequest, requestType string, v interface{}) (interface{}, error) {
	return o.ExecuteWithoutIteration(z, requestType, v)
}

// ExecuteWithoutIteration does not attempt to perform any nextTag style iteration
func (o *ZapiRunner) ExecuteWithoutIteration(z ZAPIRequest, requestType string, v interface{}) (interface{}, error) {

	if o.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": requestType}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := o.SendZapi(z)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return nil, readErr
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	//unmarshalErr := xml.Unmarshal(body, &v)
	unmarshalErr := xml.Unmarshal(body, v)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
	}
	if o.DebugTraceFlags["api"] {
		log.Debugf("%s result:\n%v", requestType, v)
	}

	return v, nil
}

// ToString implements a String() function via reflection
func ToString(val reflect.Value) string {
	if reflect.TypeOf(val).Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}

	var buffer bytes.Buffer
	if reflect.ValueOf(val).Kind() == reflect.Struct {
		for i := 0; i < val.Type().NumField(); i++ {
			fieldName := val.Type().Field(i).Name
			fieldType := val.Type().Field(i)
			fieldTag := fieldType.Tag
			fieldValue := val.Field(i)

			switch val.Field(i).Kind() {
			case reflect.Ptr:
				fieldValue = reflect.Indirect(val.Field(i))
			default:
				fieldValue = val.Field(i)
			}

			if fieldTag != "" {
				xmlTag := fieldTag.Get("xml")
				if xmlTag != "" {
					fieldName = xmlTag
				}
			}

			if fieldValue.IsValid() {
				buffer.WriteString(fmt.Sprintf("%s: %v\n", fieldName, fieldValue))
			} else {
				buffer.WriteString(fmt.Sprintf("%s: %v\n", fieldName, "nil"))
			}
		}
	}

	return buffer.String()
}
