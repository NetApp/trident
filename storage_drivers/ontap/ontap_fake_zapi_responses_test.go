package ontap

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

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

const (
	iqnRowTemplate = `               <initiator-info>
                <initiator-name>{IQN}</initiator-name>
               </initiator-info>
`
)

var (
	igroups map[string]map[string]struct{}
)

type TestZAPIResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

type TestZAPIResponse struct {
	XMLName         xml.Name               `xml:"netapp"`
	ResponseVersion string                 `xml:"version,attr"`
	ResponseXmlns   string                 `xml:"xmlns,attr"`
	Result          TestZAPIResponseResult `xml:"results"`
}

func newTestGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort string) *TestZAPIResponse {

	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
	<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
	<netapp xmlns="%s" version="%s">
	<results status="passed"/>
	</netapp>`,
		vserverAdminUrl, "1.21")

	var testZAPIResponse TestZAPIResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &testZAPIResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &testZAPIResponse
}

func newTestGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, reason, errorNum string) *TestZAPIResponse {

	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
		<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">
		<netapp xmlns="%s" version="%s">
		  <results reason="%s" status="failed" errno="%s"/>
		</netapp>`,
		vserverAdminUrl, "1.21", reason, errorNum)

	var testZAPIResponse TestZAPIResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &testZAPIResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &testZAPIResponse
}

func newTestVserverGetIterResponse(vserverAdminHost, vserverAdminPort, vserverAggrName string) *azgo.
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

func newTestVserverShowAggrGetIterResponse(vserverAdminHost, vserverAdminPort, vserverName string) *azgo.
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

func newTestIgroupGetIterResponse(vserverAdminHost, vserverAdminPort, igroupName string) *azgo.IgroupGetIterResponse {

	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"
	rows := ""
	for iqn := range igroups[igroupName] {
		rows += getIQNRowXML(iqn)
	}

	if len(igroups[igroupName]) > 1 {
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

func testGetIgroupNameFromInfo(zapiRequest string) (string, error) {

	var igroupGetIterRequest azgo.IgroupGetIterRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupGetIterRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", err
	}
	query := igroupGetIterRequest.Query()
	info := query.InitiatorGroupInfo()
	return info.InitiatorGroupName(), nil
}

func testGetIgroupNameFromCreate(zapiRequest string) (string, error) {

	var igroupCreateRequest azgo.IgroupCreateRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupCreateRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", err
	}
	return igroupCreateRequest.InitiatorGroupName(), nil
}

func testGetIgroupNameFromDestroy(zapiRequest string) (string, error) {

	var igroupDestroyRequest azgo.IgroupDestroyRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupDestroyRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", err
	}
	return igroupDestroyRequest.InitiatorGroupName(), nil
}

func testGetAddInitiator(zapiRequest string) (string, string, error) {

	var igroupAddRequest azgo.IgroupAddRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupAddRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", "", err
	}

	return igroupAddRequest.InitiatorGroupName(), igroupAddRequest.Initiator(), nil
}

func testGetRemoveInitiator(zapiRequest string) (string, string, error) {

	var igroupRemoveRequest azgo.IgroupRemoveRequest
	if err := xml.Unmarshal([]byte(zapiRequest), &igroupRemoveRequest); err != nil {
		fmt.Printf("error: %v", err.Error())
		return "", "", err
	}

	return igroupRemoveRequest.InitiatorGroupName(), igroupRemoveRequest.Initiator(), nil
}

// TODO:remove all this once the new mocking framework is in place
func testResponseObjectFactoryMethod(zapiRequestXMLTagName, vserverAdminHost, vserverAdminPort,
	vserverAggrName string, zapiRequestXMLBuilder string) ([]byte, error) {

	zapiRequestXMLBuilder = strings.Split(zapiRequestXMLBuilder, "<netapp")[1]
	zapiRequestXMLBuilder = strings.SplitN(zapiRequestXMLBuilder, ">", 2)[1]
	zapiRequestXMLBuilder = strings.Split(zapiRequestXMLBuilder, "</netapp>")[0]

	var zapiResponse interface{}
	switch zapiRequestXMLTagName {
	case "vserver-get-iter":
		zapiResponse = *newTestVserverGetIterResponse(vserverAdminHost, vserverAdminPort, vserverAggrName)
	case "vserver-show-aggr-get-iter":
		zapiResponse = *newTestVserverShowAggrGetIterResponse(vserverAdminHost, vserverAdminPort, "datavserver")
	case "igroup-create":
		if igroups == nil {
			igroups = map[string]map[string]struct{}{}
		}
		igroupName, err := testGetIgroupNameFromCreate(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" {
			zapiResponse = *newTestGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if _, ok := igroups[igroupName]; !ok {
				igroups[igroupName] = map[string]struct{}{}
			}
			zapiResponse = *newTestGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "igroup-destroy":
		igroupName, err := testGetIgroupNameFromDestroy(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" {
			zapiResponse = *newTestGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if igroups != nil {
				delete(igroups, igroupName)
			}
			zapiResponse = *newTestGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "igroup-get-iter":
		igroupName, err := testGetIgroupNameFromInfo(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" {
			zapiResponse = *newTestGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			zapiResponse = *newTestIgroupGetIterResponse(vserverAdminHost, vserverAdminPort, igroupName)
		}
	case "igroup-add":
		igroupName, iqn, err := testGetAddInitiator(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" || iqn == "" {
			zapiResponse = *newTestGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if igroups == nil {
				igroups = map[string]map[string]struct{}{}
			}
			if _, ok := igroups[igroupName]; !ok {
				igroups[igroupName] = map[string]struct{}{}
			}
			igroups[igroupName][iqn] = struct{}{}
			zapiResponse = *newTestGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	case "igroup-remove":
		igroupName, iqn, err := testGetRemoveInitiator(zapiRequestXMLBuilder)
		if err != nil || igroupName == "" || iqn == "" {
			zapiResponse = *newTestGetErrorZAPIResponse(vserverAdminHost, vserverAdminPort, "Invalid Input", "13115")
		} else {
			if igroups != nil {
				if _, ok := igroups[igroupName]; ok {
					delete(igroups[igroupName], iqn)
				}
			}
			zapiResponse = *newTestGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
		}
	default:
		zapiResponse = *newTestGetDefaultZAPIResponse(vserverAdminHost, vserverAdminPort)
	}

	output, err := xml.MarshalIndent(zapiResponse, "  ", "    ")
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	return output, nil
}

func findNextXMLStartElement(newReader io.Reader) (string, error) {
	d := xml.NewDecoder(newReader)

	for {
		token, err := d.Token()
		if err != nil {
			return "", err
		}

		if start, ok := token.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
	return "", nil
}

func getTestResponse(ctx context.Context, requestBody io.Reader, vserverAdminHost,
	vserverAdminPort, vserverAggrName string) ([]byte, error) {

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

	responseBytes, err := testResponseObjectFactoryMethod(startElement, vserverAdminHost, vserverAdminPort,
		vserverAggrName, string(requestBodyString))

	return responseBytes, nil
}

func newUnstartedVserver(ctx context.Context, vserverAdminHost, vserverAggrName string) *httptest.Server {
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	listener, err := net.Listen("tcp", vserverAdminHost+":0")
	if err != nil {
		Logc(ctx).Fatal(err)
	}
	server.Listener = listener
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	mux.HandleFunc("/servlets/", func(w http.ResponseWriter, r *http.Request) {

		response, err := getTestResponse(ctx, r.Body, vserverAdminHost, port, vserverAggrName)
		if err != nil {
			w.Write([]byte(err.Error()))
		}

		w.Write(response)
	})
	return server
}
