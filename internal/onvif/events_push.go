package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
)

const (
	pushStartDelay    = 100 * time.Millisecond
	pushHTTPTimeout   = 5 * time.Second
	pushRetryDelay    = 200 * time.Millisecond
	pushDeliveryTries = 3
	pushNotifyAction  = "http://docs.oasis-open.org/wsn/bw-2/NotificationConsumer/Notify"
)

var eventPushHTTPClient = &http.Client{
	Timeout: pushHTTPTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func eventConsumerURL(request []byte, remoteAddr string) (string, error) {
	value := eventElementText(request, "Address", "ConsumerReference")
	if value == "" {
		return "", errors.New("onvif event consumer address is required")
	}

	u, err := url.Parse(html.UnescapeString(value))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid onvif event consumer address")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("onvif event consumer address must use http or https")
	}
	if u.User != nil {
		return "", errors.New("onvif event consumer address must not contain userinfo")
	}
	if u.Fragment != "" {
		return "", errors.New("onvif event consumer address must not contain a fragment")
	}

	consumerIP := net.ParseIP(strings.TrimSuffix(u.Hostname(), "."))
	if consumerIP == nil {
		return "", errors.New("onvif event consumer host must be an IP address")
	}

	remoteHost := remoteAddr
	if host, _, splitErr := net.SplitHostPort(remoteAddr); splitErr == nil {
		remoteHost = host
	}
	if zone := strings.LastIndexByte(remoteHost, '%'); zone >= 0 {
		remoteHost = remoteHost[:zone]
	}
	remoteIP := net.ParseIP(strings.Trim(remoteHost, "[]"))
	if remoteIP == nil || !remoteIP.Equal(consumerIP) {
		return "", errors.New("onvif event consumer IP must match the requesting client")
	}

	return u.String(), nil
}

func eventTopicFilter(request []byte) string {
	return eventElementText(request, "TopicExpression", "Filter")
}

func eventElementText(request []byte, target, parent string) string {
	decoder := xml.NewDecoder(bytes.NewReader(request))
	var stack []string

	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}

		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Local == target && containsEventElement(stack, parent) {
				var value string
				if err = decoder.DecodeElement(&value, &token); err != nil {
					return ""
				}
				return strings.TrimSpace(value)
			}
			stack = append(stack, token.Name.Local)

		case xml.EndElement:
			if len(stack) != 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func containsEventElement(stack []string, name string) bool {
	if name == "" {
		return true
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == name {
			return true
		}
	}
	return false
}

func subscribeResponse(address, id string, expiresAt time.Time) []byte {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return eventEnvelope(fmt.Sprintf(`<wsnt:SubscribeResponse xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
	<wsnt:SubscriptionReference>
		<wsa:Address>%s</wsa:Address>
		<wsa:ReferenceParameters><tev:SubscriptionId>%s</tev:SubscriptionId></wsa:ReferenceParameters>
	</wsnt:SubscriptionReference>
	<wsnt:CurrentTime>%s</wsnt:CurrentTime>
	<wsnt:TerminationTime>%s</wsnt:TerminationTime>
</wsnt:SubscribeResponse>`, escapeXML(address), escapeXML(id), now, expiresAt.UTC().Format(time.RFC3339Nano)))
}

func (m *eventManager) pushLoop(id string) {
	for {
		select {
		case <-m.stop:
			return
		default:
		}

		notifications, _, err := m.pull(id, maxPullMessages, maxPullTimeout)
		if err != nil {
			return
		}
		if len(notifications) == 0 {
			continue
		}

		consumerURL, done, ok := m.pushConsumer(id)
		if !ok {
			return
		}
		ctx, cancel := eventPushContext(done, m.stop)
		err = deliverEventNotifications(ctx, consumerURL, notifications)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Warn().Err(err).Str("subscription", id).Str("consumer", consumerURL).
				Int("messages", len(notifications)).Msg("[onvif] push notification failed")
			continue
		}

		log.Info().Str("subscription", id).Str("consumer", consumerURL).
			Int("messages", len(notifications)).Msg("[onvif] push notification delivered")
		logDeliveredEvents("push", id, notifications)
	}
}

func (m *eventManager) pushConsumer(id string) (string, <-chan struct{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscriptionLocked(id, time.Now().UTC())
	if !ok || sub.ConsumerURL == "" {
		return "", nil, false
	}
	return sub.ConsumerURL, sub.Done, true
}

func eventPushContext(done, stop <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-done:
			cancel()
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func deliverEventNotifications(ctx context.Context, consumerURL string, notifications []eventNotification) error {
	body := notifyEnvelope(consumerURL, notifications)
	var lastErr error

	for attempt := 1; attempt <= pushDeliveryTries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, consumerURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8; action="`+pushNotifyAction+`"`)
		req.Header.Set("SOAPAction", `"`+pushNotifyAction+`"`)

		resp, err := eventPushHTTPClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
				return nil
			}
			err = fmt.Errorf("consumer returned HTTP %s", resp.Status)
		}
		lastErr = err

		if attempt < pushDeliveryTries {
			timer := time.NewTimer(pushRetryDelay * time.Duration(attempt))
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	return lastErr
}

func notifyEnvelope(consumerURL string, notifications []eventNotification) []byte {
	var body strings.Builder
	body.Grow(1024 + len(notifications)*512)
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:tns1="http://www.onvif.org/ver10/topics"><s:Header>`)
	body.WriteString(`<wsa:Action s:mustUnderstand="1">`)
	body.WriteString(pushNotifyAction)
	body.WriteString(`</wsa:Action><wsa:MessageID>urn:uuid:`)
	body.WriteString(pkgonvif.UUID())
	body.WriteString(`</wsa:MessageID><wsa:To s:mustUnderstand="1">`)
	body.WriteString(escapeXML(consumerURL))
	body.WriteString(`</wsa:To></s:Header><s:Body><wsnt:Notify>`)
	for _, notification := range notifications {
		body.WriteString(notificationMessageXML(notification, ""))
	}
	body.WriteString(`</wsnt:Notify></s:Body></s:Envelope>`)
	return []byte(body.String())
}
