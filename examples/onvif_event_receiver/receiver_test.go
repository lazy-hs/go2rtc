package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseNotifications(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:tt="http://www.onvif.org/ver10/schema"><s:Body><wsnt:Notify><wsnt:NotificationMessage><wsnt:Topic>tns1:VideoSource/MotionAlarm</wsnt:Topic><wsnt:Message><tt:Message UtcTime="2026-08-06T01:02:03Z" PropertyOperation="Changed"><tt:Source><tt:SimpleItem Name="VideoSourceConfigurationToken" Value="main"/></tt:Source><tt:Key><tt:SimpleItem Name="Rule" Value="Motion1"/></tt:Key><tt:Data><tt:SimpleItem Name="IsMotion" Value="true"/></tt:Data></tt:Message></wsnt:Message></wsnt:NotificationMessage></wsnt:Notify></s:Body></s:Envelope>`)

	events, err := parseNotifications(body, "push")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "push", events[0].Mode)
	require.Equal(t, "tns1:VideoSource/MotionAlarm", events[0].Topic)
	require.Equal(t, "Changed", events[0].PropertyOperation)
	require.Equal(t, []simpleItem{{Name: "VideoSourceConfigurationToken", Value: "main"}}, events[0].Source)
	require.Equal(t, []simpleItem{{Name: "IsMotion", Value: "true"}}, events[0].Data)
}

func TestParseSubscriptionResponse(t *testing.T) {
	body := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:tev="http://www.onvif.org/ver10/events/wsdl"><s:Body><tev:CreatePullPointSubscriptionResponse><tev:SubscriptionReference><wsa:Address>http://camera/onvif/events/subscription/1</wsa:Address><wsa:ReferenceParameters><tev:SubscriptionId>abc-123</tev:SubscriptionId></wsa:ReferenceParameters></tev:SubscriptionReference><tev:TerminationTime>2026-08-06T01:12:03Z</tev:TerminationTime></tev:CreatePullPointSubscriptionResponse></s:Body></s:Envelope>`)

	address, headers, expires, err := parseSubscriptionResponse(body)
	require.NoError(t, err)
	require.Equal(t, "http://camera/onvif/events/subscription/1", address)
	require.Len(t, headers, 1)
	require.Contains(t, headers[0], "SubscriptionId")
	require.Contains(t, headers[0], "IsReferenceParameter")
	require.NoError(t, xml.Unmarshal([]byte(headers[0]), new(any)))
	require.Equal(t, time.Date(2026, 8, 6, 1, 12, 3, 0, time.UTC), expires)
}

func TestSOAPFaultDetection(t *testing.T) {
	notification := []byte(`<Envelope><Body><NotificationMessage><Topic>tns1:Device/Fault</Topic></NotificationMessage></Body></Envelope>`)
	require.Empty(t, soapFault(notification))

	fault := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><s:Fault><s:Reason><s:Text xml:lang="en">Action not supported</s:Text></s:Reason></s:Fault></s:Body></s:Envelope>`)
	require.Equal(t, "SOAP fault: Action not supported", soapFault(fault))
}

func TestFindEventService(t *testing.T) {
	capabilities := []byte(`<Envelope><Body><GetCapabilitiesResponse><Capabilities><Media><XAddr>http://camera/media</XAddr></Media><Events><XAddr>http://camera/events</XAddr></Events></Capabilities></GetCapabilitiesResponse></Body></Envelope>`)
	require.Equal(t, "http://camera/events", findNestedText(capabilities, "Events", "XAddr"))

	services := []byte(`<Envelope><Body><GetServicesResponse><Service><Namespace>http://www.onvif.org/ver10/media/wsdl</Namespace><XAddr>http://camera/media</XAddr></Service><Service><Namespace>` + eventNamespace + `</Namespace><XAddr>http://camera/events</XAddr></Service></GetServicesResponse></Body></Envelope>`)
	require.Equal(t, "http://camera/events", findServiceXAddr(services, eventNamespace))
}

func TestXMLDuration(t *testing.T) {
	require.Equal(t, "PT1S", xmlDuration(time.Millisecond))
	require.Equal(t, "PT16S", xmlDuration(15*time.Second+time.Millisecond))
	require.True(t, strings.HasPrefix(xmlDuration(10*time.Minute), "PT"))
}
