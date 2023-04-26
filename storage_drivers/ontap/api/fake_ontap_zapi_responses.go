// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

const (
	iqnRowTemplate = `               <initiator-info>
                <initiator-name>{IQN}</initiator-name>
               </initiator-info>
`
)

var FakeIgroups map[string]map[string]struct{}

type FakeZAPIResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

type FakeZAPIResponse struct {
	XMLName         xml.Name               `xml:"netapp"`
	ResponseVersion string                 `xml:"version,attr"`
	ResponseXmlns   string                 `xml:"xmlns,attr"`
	Result          FakeZAPIResponseResult `xml:"results"`
}

func newFakeGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort string) *FakeZAPIResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
	<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
	<netapp xmlns="%s" version="%s">
	<results status="passed"/>
	</netapp>`,
		vserverAdminUrl, "1.21")

	var fakeZAPIResponse FakeZAPIResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &fakeZAPIResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &fakeZAPIResponse
}

func newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, reason, errorNum string) *FakeZAPIResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
		<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
		<netapp xmlns="%s" version="%s">
		  <results reason="%s" status="failed" errno="%s"/>
		</netapp>`,
		vserverAdminUrl, "1.21", reason, errorNum)

	var fakeZAPIResponse FakeZAPIResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &fakeZAPIResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &fakeZAPIResponse
}

func newFakeVserverGetResponse(vserverAdminHost, vserverAdminPort, vserverAggrName string) *azgo.
	VserverGetResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
			<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">

			<netapp xmlns="%s" version="%s">
			  <results status="passed">
				<attributes>
				  <vserver-info>
					<aggr-list>
					  <aggr-name>%s</aggr-name>
					</aggr-list>
					<allowed-protocols>
					  <protocol>nfs</protocol>
					  <protocol>cifs</protocol>
					  <protocol>fcp</protocol>
					  <protocol>iscsi</protocol>
					  <protocol>ndmp</protocol>
					</allowed-protocols>
					<antivirus-on-access-policy>default</antivirus-on-access-policy>
					<comment/>
					<ipspace>Default</ipspace>
					<is-config-locked-for-changes>false</is-config-locked-for-changes>
					<is-repository-vserver>false</is-repository-vserver>
					<is-space-enforcement-logical>false</is-space-enforcement-logical>
					<is-space-reporting-logical>false</is-space-reporting-logical>
					<is-vserver-protected>false</is-vserver-protected>
					<language>c.utf_8</language>
					<max-volumes>unlimited</max-volumes>
					<name-mapping-switch>
					  <nmswitch>file</nmswitch>
					</name-mapping-switch>
					<name-server-switch>
					  <nsswitch>file</nsswitch>
					</name-server-switch>
					<operational-state>running</operational-state>
					<quota-policy>default</quota-policy>
					<root-volume>root</root-volume>
					<root-volume-aggregate>%s</root-volume-aggregate>
					<root-volume-security-style>unix</root-volume-security-style>
					<snapshot-policy>default</snapshot-policy>
					<state>running</state>
					<uuid>7b9c12f2-bfdb-11ea-b366-005056b3362c</uuid>
					<volume-delete-retention-hours>12</volume-delete-retention-hours>
					<vserver-aggr-info-list>
					  <vserver-aggr-info>
						<aggr-availsize>600764416</aggr-availsize>
						<aggr-is-cft-precommit>false</aggr-is-cft-precommit>
						<aggr-name>%s</aggr-name>
					  </vserver-aggr-info>
					</vserver-aggr-info-list>
					<vserver-name>datavserver</vserver-name>
					<vserver-subtype>default</vserver-subtype>
					<vserver-type>data</vserver-type>
				  </vserver-info>
				</attributes>
			  </results>
			</netapp>`,
		vserverAdminUrl, "1.21", vserverAggrName, vserverAggrName, vserverAggrName)

	var vserverGetResponse azgo.VserverGetResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &vserverGetResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &vserverGetResponse
}

func newFakeVserverGetIterResponse(vserverAdminHost, vserverAdminPort, vserverAggrName string) *azgo.
	VserverGetIterResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
			<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">

			<netapp xmlns="%s" version="%s">
			  <results status="passed">
				<attributes-list>
				  <vserver-info>
					<aggr-list>
					  <aggr-name>%s</aggr-name>
					</aggr-list>
					<allowed-protocols>
					  <protocol>nfs</protocol>
					  <protocol>cifs</protocol>
					  <protocol>fcp</protocol>
					  <protocol>iscsi</protocol>
					  <protocol>ndmp</protocol>
					</allowed-protocols>
					<antivirus-on-access-policy>default</antivirus-on-access-policy>
					<comment/>
					<ipspace>Default</ipspace>
					<is-config-locked-for-changes>false</is-config-locked-for-changes>
					<is-repository-vserver>false</is-repository-vserver>
					<is-space-enforcement-logical>false</is-space-enforcement-logical>
					<is-space-reporting-logical>false</is-space-reporting-logical>
					<is-vserver-protected>false</is-vserver-protected>
					<language>c.utf_8</language>
					<max-volumes>unlimited</max-volumes>
					<name-mapping-switch>
					  <nmswitch>file</nmswitch>
					</name-mapping-switch>
					<name-server-switch>
					  <nsswitch>file</nsswitch>
					</name-server-switch>
					<operational-state>running</operational-state>
					<quota-policy>default</quota-policy>
					<root-volume>root</root-volume>
					<root-volume-aggregate>%s</root-volume-aggregate>
					<root-volume-security-style>unix</root-volume-security-style>
					<snapshot-policy>default</snapshot-policy>
					<state>running</state>
					<uuid>7b9c12f2-bfdb-11ea-b366-005056b3362c</uuid>
					<volume-delete-retention-hours>12</volume-delete-retention-hours>
					<vserver-aggr-info-list>
					  <vserver-aggr-info>
						<aggr-availsize>600764416</aggr-availsize>
						<aggr-is-cft-precommit>false</aggr-is-cft-precommit>
						<aggr-name>%s</aggr-name>
					  </vserver-aggr-info>
					</vserver-aggr-info-list>
					<vserver-name>datavserver</vserver-name>
					<vserver-subtype>default</vserver-subtype>
					<vserver-type>data</vserver-type>
				  </vserver-info>
				</attributes-list>
				<num-records>1</num-records>
			  </results>
			</netapp>`,
		vserverAdminUrl, "1.21", vserverAggrName, vserverAggrName, vserverAggrName)

	var vserverGetIterResponse azgo.VserverGetIterResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &vserverGetIterResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &vserverGetIterResponse
}

func newFakeVserverShowAggrGetIterResponse(vserverAdminHost, vserverAdminPort, vserverName string) *azgo.
	VserverShowAggrGetIterResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
		<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
		<netapp xmlns="%s" version="%s">
		  <results status="passed">
			<attributes-list>
			  <show-aggregates>
				<aggregate-name>data</aggregate-name>
				<aggregate-type>vmdisk</aggregate-type>
				<available-size>1313234944</available-size>
				<is-nve-capable>false</is-nve-capable>
				<snaplock-type>non_snaplock</snaplock-type>
				<vserver-name>%s</vserver-name>
			  </show-aggregates>
			</attributes-list>
			<num-records>1</num-records>
		  </results>
		</netapp>`,
		vserverAdminUrl, "1.21", vserverName)

	var vserverShowAggrGetIterResponse azgo.VserverShowAggrGetIterResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &vserverShowAggrGetIterResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &vserverShowAggrGetIterResponse
}

func getIQNRowXML(iqn string) string {
	iqnRowXML := strings.Replace(iqnRowTemplate, "{IQN}", iqn, -1)
	return iqnRowXML
}

func newFakeIgroupGetIterResponse(vserverAdminHost, vserverAdminPort, igroupName string) *azgo.IgroupGetIterResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"
	rows := ""
	for iqn := range FakeIgroups[igroupName] {
		rows += getIQNRowXML(iqn)
	}

	if len(FakeIgroups[igroupName]) > 1 {
		rows = `<initiators>
` + rows + `
</initiators>
`
	}

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
	<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
	<netapp xmlns="%s" version="%s">
	  <results status="passed">
		<attributes-list>
		  <initiator-group-info>
			<initiator-group-alua-enabled>true</initiator-group-alua-enabled>
			<initiator-group-delete-on-unmap>false</initiator-group-delete-on-unmap>
			<initiator-group-name>%s</initiator-group-name>
			<initiator-group-os-type>linux</initiator-group-os-type>
			<initiator-group-throttle-borrow>false</initiator-group-throttle-borrow>
			<initiator-group-throttle-reserve>0</initiator-group-throttle-reserve>
			<initiator-group-type>iscsi</initiator-group-type>
			<initiator-group-use-partner>true</initiator-group-use-partner>
			<initiator-group-uuid>fake-igroup-UUID</initiator-group-uuid>
			<initiator-group-vsa-enabled>false</initiator-group-vsa-enabled>
			%s
			<vserver>nazneen</vserver>
		  </initiator-group-info>
		</attributes-list>
		<num-records>1</num-records>
	  </results>
	</netapp>`,
		vserverAdminUrl, "1.21", igroupName, rows)

	var igroupGetIterResponse azgo.IgroupGetIterResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &igroupGetIterResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &igroupGetIterResponse
}

func newFakeSystemNodeGetIterResponse(vserverAdminHost, vserverAdminPort string) *azgo.SystemNodeGetIterResponse {
	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
	<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
	<netapp xmlns="%s" version="%s">
	  <results status="passed">
		<attributes-list>
		  <node-details-info>
			<node>foobar</node>
		  </node-details-info>
		  <node-details-info>
			<node>baz</node>
			<node-serial-number>1234567890AB</node-serial-number>
          </node-details-info>
		</attributes-list>
		<num-records>2</num-records>
	  </results>
	</netapp>`,
		vserverAdminUrl, "1.21")

	var systemNodeGetIterResponse azgo.SystemNodeGetIterResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &systemNodeGetIterResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &systemNodeGetIterResponse
}

func fakeGetIgroupNameFromInfo(zapiRequest string) (string, error) {
	var igroupGetIterRequest azgo.IgroupGetIterRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupGetIterRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", err
	}
	if igroupGetIterRequest.QueryPtr == nil {
		return "", nil
	}
	query := igroupGetIterRequest.Query()
	if query.InitiatorGroupInfoPtr == nil {
		return "", nil
	}
	info := query.InitiatorGroupInfo()
	if info.InitiatorGroupNamePtr == nil {
		return "", nil
	}
	return info.InitiatorGroupName(), nil
}

func fakeGetIgroupNameFromCreate(zapiRequest string) (string, error) {
	var igroupCreateRequest azgo.IgroupCreateRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupCreateRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", err
	}

	if igroupCreateRequest.InitiatorGroupNamePtr == nil {
		return "", nil
	}
	return igroupCreateRequest.InitiatorGroupName(), nil
}

func fakeGetIgroupNameFromDestroy(zapiRequest string) (string, error) {
	var igroupDestroyRequest azgo.IgroupDestroyRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupDestroyRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", err
	}

	if igroupDestroyRequest.InitiatorGroupNamePtr == nil {
		return "", nil
	}
	return igroupDestroyRequest.InitiatorGroupName(), nil
}

func fakeGetAddInitiator(zapiRequest string) (string, string, error) {
	var igroupAddRequest azgo.IgroupAddRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupAddRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", "", err
	}

	if igroupAddRequest.InitiatorGroupNamePtr == nil || igroupAddRequest.InitiatorPtr == nil {
		return "", "", nil
	}
	return igroupAddRequest.InitiatorGroupName(), igroupAddRequest.Initiator(), nil
}

func fakeGetRemoveInitiator(zapiRequest string) (string, string, error) {
	var igroupRemoveRequest azgo.IgroupRemoveRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupRemoveRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", "", err
	}

	if igroupRemoveRequest.InitiatorGroupNamePtr == nil || igroupRemoveRequest.InitiatorPtr == nil {
		return "", "", nil
	}
	return igroupRemoveRequest.InitiatorGroupName(), igroupRemoveRequest.Initiator(), nil
}

func fakeSystemNodeGetIter(zapiRequest string) error {
	var systemNodeGetIterRequest azgo.SystemNodeGetIterRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &systemNodeGetIterRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return err
	}

	return nil
}

// TODO:remove all this once the new mocking framework is in place
func fakeResponseObjectFactoryMethod(zapiRequestXMLTagName, vserverAdminHost, vserverAdminPort,
	vserverAggrName string, zapiRequestXMLBuilder string,
) ([]byte, error) {
	zapiRequestXMLBuilder = strings.Split(zapiRequestXMLBuilder, "<netapp")[1]
	zapiRequestXMLBuilder = strings.SplitN(zapiRequestXMLBuilder, ">", 2)[1]
	zapiRequestXMLBuilder = strings.Split(zapiRequestXMLBuilder, "</netapp>")[0]

	var zapiResponse interface{}
	switch zapiRequestXMLTagName {
	case "vserver-get":
		zapiResponse = *newFakeVserverGetResponse(vserverAdminHost, vserverAdminPort, vserverAggrName)
	case "vserver-get-iter":
		zapiResponse = *newFakeVserverGetIterResponse(vserverAdminHost, vserverAdminPort, vserverAggrName)
	case "vserver-show-aggr-get-iter":
		zapiResponse = *newFakeVserverShowAggrGetIterResponse(vserverAdminHost, vserverAdminPort, "datavserver")
	case "igroup-create":
		if FakeIgroups == nil {
			FakeIgroups = map[string]map[string]struct{}{}
		}
		igroupName, err := fakeGetIgroupNameFromCreate(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" {
			zapiResponse = *newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if _, ok := FakeIgroups[igroupName]; !ok {
				FakeIgroups[igroupName] = map[string]struct{}{}
			}
			zapiResponse = *newFakeGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "igroup-destroy":
		igroupName, err := fakeGetIgroupNameFromDestroy(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" {
			zapiResponse = *newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if FakeIgroups != nil {
				delete(FakeIgroups, igroupName)
			}
			zapiResponse = *newFakeGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "igroup-get-iter":
		igroupName, err := fakeGetIgroupNameFromInfo(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" {
			zapiResponse = *newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			zapiResponse = *newFakeIgroupGetIterResponse(vserverAdminHost, vserverAdminPort, igroupName)
		}
	case "igroup-add":
		igroupName, iqn, err := fakeGetAddInitiator(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" || iqn == "" {
			zapiResponse = *newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if FakeIgroups == nil {
				FakeIgroups = map[string]map[string]struct{}{}
			}
			if _, ok := FakeIgroups[igroupName]; !ok {
				FakeIgroups[igroupName] = map[string]struct{}{}
			}
			FakeIgroups[igroupName][iqn] = struct{}{}
			zapiResponse = *newFakeGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "igroup-remove":
		igroupName, iqn, err := fakeGetRemoveInitiator(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" || iqn == "" {
			zapiResponse = *newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if FakeIgroups != nil {
				if _, ok := FakeIgroups[igroupName]; ok {
					delete(FakeIgroups[igroupName], iqn)
				}
			}
			zapiResponse = *newFakeGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "system-node-get-iter":
		err := fakeSystemNodeGetIter(zapiRequestXMLBuilder)
		if err != nil {
			zapiResponse = *newFakeGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			zapiResponse = *newFakeSystemNodeGetIterResponse(vserverAdminHost, vserverAdminPort)
		}
	default:
		zapiResponse = *newFakeGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
	}

	output, err := xml.MarshalIndent(zapiResponse, "  ", "    ")
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	return output, nil
}

func findNextXMLStartElement(newReader io.Reader) (string, error) {
	d := xml.NewDecoder(newReader)
	var start xml.StartElement
	var ok bool

	for {
		token, err := d.Token()
		if err != nil {
			return "", err
		}

		if start, ok = token.(xml.StartElement); ok {
			break
		}
	}
	return start.Name.Local, nil
}

func getFakeResponse(ctx context.Context, requestBody io.Reader, vserverAdminHost,
	vserverAdminPort, vserverAggrName string,
) ([]byte, error) {
	requestBodyString, err := ioutil.ReadAll(requestBody)
	if err != nil {
		Logc(ctx).WithField("zapi request: ", string(requestBodyString))
		return nil, err
	}

	newReader := bytes.NewReader(requestBodyString)

	startElement, err := findNextXMLStartElement(newReader)

	if err == nil && startElement == "netapp" {
		startElement, err = findNextXMLStartElement(newReader)
	}

	if err != nil {
		return nil, err
	}

	responseBytes, err := fakeResponseObjectFactoryMethod(startElement, vserverAdminHost, vserverAdminPort,
		vserverAggrName, string(requestBodyString))

	return responseBytes, nil
}

func NewFakeUnstartedVserver(ctx context.Context, vserverAdminHost, vserverAggrName string) *httptest.Server {
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	listener, err := net.Listen("tcp", vserverAdminHost+":0")
	if err != nil {
		Logc(ctx).Fatal(err)
	}
	server.Listener = listener
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	mux.HandleFunc("/servlets/", func(w http.ResponseWriter, r *http.Request) {
		response, err := getFakeResponse(ctx, r.Body, vserverAdminHost, port, vserverAggrName)
		if err != nil {
			if _, err := w.Write([]byte(err.Error())); err != nil {
				Logc(ctx).WithError(err).Error("fake HTTP response write failure.")
			}
		}

		_, err = w.Write(response)
		if err != nil {
			Logc(ctx).WithError(err).Error("fake HTTP response write failure.")
		}
	})
	return server
}
