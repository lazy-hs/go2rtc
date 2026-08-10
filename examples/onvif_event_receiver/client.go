package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

const (
	wsaNamespace   = "http://www.w3.org/2005/08/addressing"
	wsaAnonymous   = wsaNamespace + "/anonymous"
	eventNamespace = "http://www.onvif.org/ver10/events/wsdl"
	wsntNamespace  = "http://docs.oasis-open.org/wsn/b-2"

	actionGetCapabilities = "http://www.onvif.org/ver10/device/wsdl/GetCapabilities"
	actionGetServices     = "http://www.onvif.org/ver10/device/wsdl/GetServices"
	actionSubscribe       = "http://docs.oasis-open.org/wsn/bw-2/NotificationProducer/SubscribeRequest"
	actionCreatePullPoint = eventNamespace + "/EventPortType/CreatePullPointSubscriptionRequest"
	actionPullMessages    = eventNamespace + "/PullPointSubscription/PullMessagesRequest"
	actionRenew           = "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/RenewRequest"
	actionUnsubscribe     = "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest"
)

type eventClient struct {
	deviceURL *url.URL
	eventURL  *url.URL
	user      *url.Userinfo
}

type subscription struct {
	manager *url.URL
	headers []string
	expires time.Time
}

func newEventClient(deviceRaw, eventRaw string) (*eventClient, error) {
	deviceURL, err := parseEndpoint(deviceRaw, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid device URL: %w", err)
	}
	if deviceURL.Path == "" || deviceURL.Path == "/" {
		deviceURL.Path = onvif.PathDevice
	}

	client := &eventClient{deviceURL: deviceURL, user: deviceURL.User}
	if eventRaw != "" {
		client.eventURL, err = parseEndpoint(eventRaw, client.user)
		if err != nil {
			return nil, fmt.Errorf("invalid event service URL: %w", err)
		}
	}
	return client, nil
}

func parseEndpoint(raw string, inheritedUser *url.Userinfo) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.Scheme == "onvif" {
		u.Scheme = "http"
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "httpx" {
		return nil, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("URL host is empty")
	}
	if u.User == nil {
		u.User = inheritedUser
	}
	return u, nil
}

func (c *eventClient) discover(ctx context.Context) error {
	if c.eventURL != nil {
		return nil
	}

	body, capabilityErr := c.soap(ctx, c.deviceURL, actionGetCapabilities,
		`<tds:GetCapabilities><tds:Category>Events</tds:Category></tds:GetCapabilities>`, nil)
	if capabilityErr == nil {
		if xaddr := findNestedText(body, "Events", "XAddr"); xaddr != "" {
			var err error
			c.eventURL, err = c.resolveEndpoint(xaddr, c.deviceURL)
			return err
		}
	}

	body, servicesErr := c.soap(ctx, c.deviceURL, actionGetServices,
		`<tds:GetServices><tds:IncludeCapability>false</tds:IncludeCapability></tds:GetServices>`, nil)
	if servicesErr == nil {
		if xaddr := findServiceXAddr(body, eventNamespace); xaddr != "" {
			var err error
			c.eventURL, err = c.resolveEndpoint(xaddr, c.deviceURL)
			return err
		}
	}

	if capabilityErr != nil {
		return fmt.Errorf("discover Events service: GetCapabilities: %v; GetServices: %v", capabilityErr, servicesErr)
	}
	return errors.New("camera response does not advertise an ONVIF Events service")
}

func (c *eventClient) resolveEndpoint(raw string, base *url.URL) (*url.URL, error) {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	if u.Hostname() == "0.0.0.0" {
		u.Host = strings.Replace(u.Host, "0.0.0.0", base.Hostname(), 1)
	}
	if u.User == nil {
		u.User = c.user
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "httpx" {
		return nil, fmt.Errorf("unsupported service URL scheme %q", u.Scheme)
	}
	return u, nil
}

func (c *eventClient) soap(ctx context.Context, endpoint *url.URL, action, operation string, referenceHeaders []string) ([]byte, error) {
	headers := []string{
		`<wsa:Action xmlns:wsa="` + wsaNamespace + `" s:mustUnderstand="1">` + escapeXML(action) + `</wsa:Action>`,
		`<wsa:MessageID xmlns:wsa="` + wsaNamespace + `">urn:uuid:` + onvif.UUID() + `</wsa:MessageID>`,
		`<wsa:ReplyTo xmlns:wsa="` + wsaNamespace + `"><wsa:Address>` + wsaAnonymous + `</wsa:Address></wsa:ReplyTo>`,
		`<wsa:To xmlns:wsa="` + wsaNamespace + `" s:mustUnderstand="1">` + escapeXML(displayURL(endpoint)) + `</wsa:To>`,
	}
	headers = append(headers, referenceHeaders...)

	envelope := onvif.NewEnvelopeWithHeaders(c.user, headers...)
	envelope.Append(operation)
	payload := envelope.Bytes()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8; action="`+action+`"`)
	req.Header.Set("SOAPAction", `"`+action+`"`)

	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %s: %s", res.Status, soapFault(body))
	}
	if fault := soapFault(body); fault != "" {
		return nil, errors.New(fault)
	}
	return body, nil
}

func (c *eventClient) parseSubscription(body []byte) (*subscription, error) {
	address, headers, expires, err := parseSubscriptionResponse(body)
	if err != nil {
		return nil, err
	}
	manager, err := c.resolveEndpoint(address, c.eventURL)
	if err != nil {
		return nil, fmt.Errorf("invalid subscription manager URL: %w", err)
	}
	return &subscription{manager: manager, headers: headers, expires: expires}, nil
}

func displayURL(u *url.URL) string {
	copyURL := *u
	copyURL.User = nil
	return copyURL.String()
}

func escapeXML(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func soapFault(body []byte) string {
	if !hasElement(body, "Fault") {
		return ""
	}
	if reason := findNestedText(body, "Fault", "Text"); reason != "" {
		return "SOAP fault: " + strings.TrimSpace(reason)
	}
	if reason := findNestedText(body, "Fault", "faultstring"); reason != "" {
		return "SOAP fault: " + strings.TrimSpace(reason)
	}
	return "SOAP fault"
}
