package onvif

import (
	"context"
	"encoding/xml"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEventConfigFormats(t *testing.T) {
	t.Run("mapping", func(t *testing.T) {
		var cfg struct {
			Event eventConfig `yaml:"event"`
		}
		err := yaml.Unmarshal([]byte(`event:
  interval: 5s
  burst: 3
  permanent: true
  templates:
    - topic: tns1:VideoSource/MotionAlarm
      startData: start
      endData: end
`), &cfg)
		require.NoError(t, err)
		require.Equal(t, "5s", cfg.Event.Interval)
		require.Equal(t, 3, cfg.Event.Burst)
		require.True(t, cfg.Event.Permanent)
		require.Len(t, cfg.Event.Templates, 1)
	})

	t.Run("template list", func(t *testing.T) {
		var cfg struct {
			Event eventConfig `yaml:"event"`
		}
		err := yaml.Unmarshal([]byte(`event:
  - topic: tns1:VideoSource/MotionAlarm
    startData: start
    endData: end
`), &cfg)
		require.NoError(t, err)
		require.Equal(t, defaultEventInterval.String(), cfg.Event.Interval)
		require.Equal(t, defaultEventBurst, cfg.Event.Burst)
		require.Len(t, cfg.Event.Templates, 1)
	})
}

func TestEventManagerLifecycle(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval: "1h",
		Burst:    2,
		Templates: []eventTemplate{{
			Topic:          "tns1:VideoSource/MotionAlarm",
			StartData:      "start",
			EndData:        "end",
			StartOperation: "Changed",
			EndOperation:   "Deleted",
		}},
	})
	defer manager.close()

	_, id := manager.create("main", time.Minute)
	notifications, _, err := manager.pull(id, 10, 0)
	require.NoError(t, err)
	require.Len(t, notifications, 2)
	require.Equal(t, "start", notifications[0].Data)
	require.Equal(t, "end", notifications[1].Data)
	require.Equal(t, "main", notifications[0].Source)
	require.Equal(t, "Changed", notifications[0].Operation)
	require.Equal(t, "Deleted", notifications[1].Operation)
	require.False(t, notifications[0].StartTime.IsZero())
	require.True(t, notifications[0].EndTime.IsZero())
	require.Equal(t, notifications[0].StartTime, notifications[1].StartTime)
	require.Equal(t, notifications[1].Time, notifications[1].EndTime)
	require.GreaterOrEqual(t, notifications[1].EndTime.Sub(notifications[1].StartTime), minimumEventDuration)

	expiresAt, err := manager.renew(id, 2*time.Minute)
	require.NoError(t, err)
	require.True(t, expiresAt.After(time.Now().Add(time.Minute)))

	require.NoError(t, manager.synchronize(id))
	notifications, _, err = manager.pull(id, 10, 0)
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	require.Equal(t, "Initialized", notifications[0].Operation)

	require.NoError(t, manager.unsubscribe(id))
	require.NoError(t, manager.unsubscribe(id))
	_, _, err = manager.pull(id, 10, 0)
	require.ErrorIs(t, err, errSubscriptionNotFound)
}

func TestEventNotificationIncludesLifecycleTimes(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval: "1h",
		Templates: []eventTemplate{{
			Topic:          "tns1:VideoSource/MotionAlarm",
			StartData:      `<tt:SimpleItem Value="true" Name="IsMotion"/>`,
			EndData:        `<tt:SimpleItem Value="false" Name="IsMotion"/>`,
			StartOperation: "Changed",
			EndOperation:   "Deleted",
		}},
	})
	defer manager.close()

	startTime := time.Date(2026, time.August, 20, 10, 30, 0, 0, time.UTC)
	endTime := startTime.Add(15 * time.Second)
	started := manager.nextNotificationLocked(0, startTime)
	ended := manager.nextNotificationLocked(0, endTime)

	require.Equal(t, startTime, started.Time)
	require.Equal(t, startTime, started.StartTime)
	require.True(t, started.EndTime.IsZero())
	require.Equal(t, endTime, ended.Time)
	require.Equal(t, startTime, ended.StartTime)
	require.Equal(t, endTime, ended.EndTime)

	startedXML := notificationMessageXML(started, "")
	require.Contains(t, startedXML, `UtcTime="2026-08-20T10:30:00Z"`)
	require.Contains(t, startedXML, `Name="StartTime" Value="2026-08-20T10:30:00Z"`)
	require.NotContains(t, startedXML, `Name="EndTime"`)

	endedXML := notificationMessageXML(ended, "")
	require.Contains(t, endedXML, `UtcTime="2026-08-20T10:30:15Z"`)
	require.Contains(t, endedXML, `Name="StartTime" Value="2026-08-20T10:30:00Z"`)
	require.Contains(t, endedXML, `Name="EndTime" Value="2026-08-20T10:30:15Z"`)
}

func TestEventEndTimeIsNeverEqualToStartTime(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval: "1h",
		Templates: []eventTemplate{{
			Topic:     "tns1:VideoSource/MotionAlarm",
			StartData: "start",
			EndData:   "end",
		}},
	})
	defer manager.close()

	now := time.Date(2026, time.August, 20, 10, 30, 0, 0, time.UTC)
	started := manager.nextNotificationLocked(0, now)
	ended := manager.nextNotificationLocked(0, now)

	require.Equal(t, now, started.StartTime)
	require.Equal(t, now.Add(minimumEventDuration), ended.EndTime)
	require.GreaterOrEqual(t, ended.EndTime.Sub(ended.StartTime), minimumEventDuration)
}

func TestEventManagerDisabledDoesNotQueueNotifications(t *testing.T) {
	disabled := false
	manager := newEventManager(eventConfig{
		Enabled:  &disabled,
		Interval: "1h",
		Burst:    2,
		Templates: []eventTemplate{{
			Topic:     "tns1:VideoSource/MotionAlarm",
			StartData: `<tt:SimpleItem Value="true" Name="IsMotion"/>`,
		}},
	})
	defer manager.close()

	_, id := manager.create("main", time.Minute)
	notifications, _, err := manager.pull(id, 10, 0)
	require.NoError(t, err)
	require.Empty(t, notifications)

	require.NoError(t, manager.synchronize(id))
	notifications, _, err = manager.pull(id, 10, 0)
	require.NoError(t, err)
	require.Empty(t, notifications)
}

func TestEventBroadcastsToEveryMatchingSubscription(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval: "1h",
		Burst:    0,
		Templates: []eventTemplate{{
			Topic:          "tns1:VideoSource/MotionAlarm",
			StartData:      "start",
			EndData:        "end",
			StartOperation: "Changed",
			EndOperation:   "Deleted",
		}},
	})
	defer manager.close()

	_, firstID := manager.createPull("main", "tns1:VideoSource/MotionAlarm", time.Minute)
	_, secondID := manager.createPull("main", "tns1:VideoSource/MotionAlarm", time.Minute)
	_, unmatchedID := manager.createPull("main", "tns1:Device/HardwareFailure", time.Minute)

	now := time.Now().UTC()
	manager.generate(now)

	first, _, err := manager.pull(firstID, 10, 0)
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, _, err := manager.pull(secondID, 10, 0)
	require.NoError(t, err)
	require.Equal(t, first, second)

	unmatched, _, err := manager.pull(unmatchedID, 10, 0)
	require.NoError(t, err)
	require.Empty(t, unmatched)

	// Pulling from one IP/subscription must not drain another subscription queue.
	manager.generate(now.Add(time.Second))
	first, _, err = manager.pull(firstID, 10, 0)
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, _, err = manager.pull(secondID, 10, 0)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestEventBroadcastCoversDifferentTopicFilters(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval: "1h",
		Burst:    0,
		Templates: []eventTemplate{
			{Topic: "tns1:VideoSource/MotionAlarm", StartData: "motion"},
			{Topic: "tns1:Device/HardwareFailure", StartData: "hardware"},
		},
	})
	defer manager.close()

	_, motionID := manager.createPull("main", "tns1:VideoSource/MotionAlarm", time.Minute)
	_, hardwareID := manager.createPull("main", "tns1:Device/HardwareFailure", time.Minute)
	_, allID := manager.createPull("main", "", time.Minute)

	manager.generate(time.Now().UTC())

	motion, _, err := manager.pull(motionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, motion, 1)
	require.Equal(t, "tns1:VideoSource/MotionAlarm", motion[0].Topic)

	hardware, _, err := manager.pull(hardwareID, 10, 0)
	require.NoError(t, err)
	require.Len(t, hardware, 1)
	require.Equal(t, "tns1:Device/HardwareFailure", hardware[0].Topic)

	all, _, err := manager.pull(allID, 10, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestDisabledEventTemplateIsFiltered(t *testing.T) {
	disabled := false
	manager := newEventManager(eventConfig{
		Interval: "1h",
		Burst:    1,
		Templates: []eventTemplate{
			{Enabled: &disabled, Topic: "tns1:VideoSource/MotionAlarm"},
			{Topic: "tns1:VideoAnalytics/Vehicle"},
		},
	})
	defer manager.close()

	sub, id := manager.createPull("main", "", time.Minute)
	require.Equal(t, []int{1}, sub.TemplateIndexes)
	notifications, _, err := manager.pull(id, 10, 0)
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	require.Equal(t, "tns1:VideoAnalytics/Vehicle", notifications[0].Topic)
}

func TestEventSubscriptionExpiresWhileWaiting(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval:  "1h",
		Templates: []eventTemplate{{Topic: "tns1:VideoSource/MotionAlarm"}},
	})
	defer manager.close()

	_, id := manager.createPull("main", "", 50*time.Millisecond)
	started := time.Now()
	_, _, err := manager.pull(id, 10, 2*time.Second)
	require.ErrorIs(t, err, errSubscriptionNotFound)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestPermanentEventSubscription(t *testing.T) {
	manager := newEventManager(eventConfig{
		Interval:  "1h",
		Permanent: true,
		Templates: []eventTemplate{{
			Topic:     "tns1:VideoSource/MotionAlarm",
			StartData: "start",
			EndData:   "end",
		}},
	})
	defer manager.close()

	sub, id := manager.create("main", time.Nanosecond)
	require.Equal(t, permanentSubscriptionExpiration, sub.ExpiresAt)

	manager.generate(time.Now().UTC().Add(24 * time.Hour))
	require.Contains(t, manager.subscriptions, id)

	expiresAt, err := manager.renew(id, time.Nanosecond)
	require.NoError(t, err)
	require.Equal(t, permanentSubscriptionExpiration, expiresAt)
}

func TestEventSOAPFlow(t *testing.T) {
	previous := events
	events = newEventManager(eventConfig{
		Interval: "1h",
		Burst:    1,
		Templates: []eventTemplate{{
			Topic:      "tns1:VideoSource/MotionAlarm",
			SourceData: `<tt:SimpleItem Value="analytics_video_source_audio_source" Name="VideoAnalyticsConfigurationToken"/><tt:SimpleItem Value="MyMotionDetectorRule" Name="Rule"/>`,
			StartData:  `<tt:SimpleItem Value="true" Name="IsMotion"/>`,
			EndData:    `<tt:SimpleItem Value="false" Name="IsMotion"/>`,
		}},
	})
	defer func() {
		events.close()
		events = previous
	}()

	createRequest := httptest.NewRequest("POST", "http://camera.local/onvif/event_service?channel=2", strings.NewReader(`<CreatePullPointSubscription><InitialTerminationTime>PT2M</InitialTerminationTime></CreatePullPointSubscription>`))
	createBody, err := eventResponse(createRequest, []byte(`<InitialTerminationTime>PT2M</InitialTerminationTime>`), eventCreatePullPointSubscription)
	require.NoError(t, err)
	require.NoError(t, xml.Unmarshal(createBody, &struct{}{}))

	address := html.UnescapeString(pkgonvif.FindTagValue(createBody, "Address"))
	require.NotEmpty(t, address)
	require.NotEmpty(t, pkgonvif.FindTagValue(createBody, "SubscriptionId"))
	subscriptionURL, err := url.Parse(address)
	require.NoError(t, err)
	require.NotEmpty(t, subscriptionURL.Query().Get("Idx"))
	require.Equal(t, "2", subscriptionURL.Query().Get("channel"))

	pullRequest := httptest.NewRequest("POST", address, strings.NewReader(`<PullMessages><Timeout>PT0S</Timeout><MessageLimit>10</MessageLimit></PullMessages>`))
	pullBody, err := eventResponse(pullRequest, []byte(`<Timeout>PT0S</Timeout><MessageLimit>10</MessageLimit>`), eventPullMessages)
	require.NoError(t, err)
	require.NoError(t, xml.Unmarshal(pullBody, &struct{}{}))
	require.Contains(t, string(pullBody), "PullMessagesResponse")
	require.Contains(t, string(pullBody), "tns1:VideoSource/MotionAlarm")
	require.Contains(t, string(pullBody), `Value="true"`)
	require.Contains(t, string(pullBody), `Name="VideoAnalyticsConfigurationToken"`)
	require.Contains(t, string(pullBody), `Name="Rule"`)

	unsubscribeRequestBody := `<s:Header><wsa:To>` + html.EscapeString(address) + `</wsa:To></s:Header>`
	unsubscribeRequest := httptest.NewRequest("POST", "http://camera.local/onvif/event_service", strings.NewReader(unsubscribeRequestBody))
	unsubscribeBody, err := eventResponse(unsubscribeRequest, []byte(unsubscribeRequestBody), eventUnsubscribe)
	require.NoError(t, err)
	require.Contains(t, string(unsubscribeBody), "UnsubscribeResponse")

	// Some ONVIF clients unsubscribe more than once during teardown.
	unsubscribeBody, err = eventResponse(unsubscribeRequest, []byte(unsubscribeRequestBody), eventUnsubscribe)
	require.NoError(t, err)
	require.Contains(t, string(unsubscribeBody), "UnsubscribeResponse")
}

func TestEventPushSubscription(t *testing.T) {
	type delivery struct {
		Action string
		Body   string
	}
	deliveries := make(chan delivery, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		deliveries <- delivery{Action: r.Header.Get("SOAPAction"), Body: string(body)}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer consumer.Close()

	previous := events
	events = newEventManager(eventConfig{
		Interval: "1h",
		Burst:    1,
		Templates: []eventTemplate{
			{
				Topic:     "tns1:VideoSource/MotionAlarm",
				StartData: `<tt:SimpleItem Value="true" Name="IsMotion"/>`,
				EndData:   `<tt:SimpleItem Value="false" Name="IsMotion"/>`,
			},
			{
				Topic:     "tns1:VideoAnalytics/Vehicle",
				StartData: `<tt:SimpleItem Value="true" Name="Vehicle"/>`,
				EndData:   `<tt:SimpleItem Value="false" Name="Vehicle"/>`,
			},
		},
	})
	defer func() {
		events.close()
		events = previous
	}()

	consumerURL, err := url.Parse(consumer.URL)
	require.NoError(t, err)
	subscribeXML := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:tns1="http://www.onvif.org/ver10/topics"><s:Body><wsnt:Subscribe><wsnt:ConsumerReference><wsa:Address>` +
		html.EscapeString(consumer.URL) +
		`</wsa:Address></wsnt:ConsumerReference><wsnt:Filter><wsnt:TopicExpression Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">tns1:VideoSource/MotionAlarm</wsnt:TopicExpression></wsnt:Filter><wsnt:InitialTerminationTime>PT2M</wsnt:InitialTerminationTime></wsnt:Subscribe></s:Body></s:Envelope>`
	subscribeRequest := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/event_service?channel=2", strings.NewReader(subscribeXML))
	subscribeRequest.RemoteAddr = consumerURL.Hostname() + ":32100"
	subscribeBody, err := eventResponse(subscribeRequest, []byte(subscribeXML), eventSubscribe)
	require.NoError(t, err)
	require.NoError(t, xml.Unmarshal(subscribeBody, &struct{}{}))
	require.Contains(t, string(subscribeBody), "SubscribeResponse")

	managerAddress := html.UnescapeString(pkgonvif.FindTagValue(subscribeBody, "Address"))
	require.NotEmpty(t, managerAddress)
	require.NotEmpty(t, pkgonvif.FindTagValue(subscribeBody, "SubscriptionId"))

	select {
	case pushed := <-deliveries:
		require.Equal(t, `"`+pushNotifyAction+`"`, pushed.Action)
		require.Contains(t, pushed.Body, "<wsnt:Notify>")
		require.Contains(t, pushed.Body, "tns1:VideoSource/MotionAlarm")
		require.NotContains(t, pushed.Body, "tns1:VideoAnalytics/Vehicle")
		require.Contains(t, pushed.Body, `Value="true"`)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ONVIF Notify")
	}

	unsubscribeRequest := httptest.NewRequest(http.MethodPost, managerAddress, strings.NewReader(`<wsnt:Unsubscribe xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"/>`))
	unsubscribeBody, err := eventResponse(unsubscribeRequest, nil, eventUnsubscribe)
	require.NoError(t, err)
	require.Contains(t, string(unsubscribeBody), "UnsubscribeResponse")
	events.generate(time.Now().UTC())

	select {
	case pushed := <-deliveries:
		t.Fatalf("received Notify after unsubscribe: %s", pushed.Body)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestEventPushBroadcastsToMultipleConsumers(t *testing.T) {
	deliveries := []chan string{make(chan string, 1), make(chan string, 1)}
	consumers := make([]*httptest.Server, 0, len(deliveries))
	for _, delivered := range deliveries {
		delivered := delivered
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			delivered <- string(body)
			w.WriteHeader(http.StatusNoContent)
		}))
		consumers = append(consumers, consumer)
		defer consumer.Close()
	}

	manager := newEventManager(eventConfig{
		Interval: "1h",
		Burst:    0,
		Templates: []eventTemplate{{
			Topic:     "tns1:VideoSource/MotionAlarm",
			StartData: `<tt:SimpleItem Value="true" Name="IsMotion"/>`,
			EndData:   `<tt:SimpleItem Value="false" Name="IsMotion"/>`,
		}},
	})
	defer manager.close()

	for _, consumer := range consumers {
		manager.createPush("main", "tns1:VideoSource/MotionAlarm", consumer.URL, time.Minute)
	}
	manager.generate(time.Now().UTC())

	for _, delivered := range deliveries {
		select {
		case body := <-delivered:
			require.Contains(t, body, "tns1:VideoSource/MotionAlarm")
			require.Contains(t, body, `Value="true"`)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for broadcast ONVIF event")
		}
	}
}

func TestEventConsumerURLValidation(t *testing.T) {
	request := []byte(`<Subscribe><ConsumerReference><Address>http://127.0.0.1:8080/events</Address></ConsumerReference></Subscribe>`)

	consumerURL, err := eventConsumerURL(request, "127.0.0.1:32000")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/events", consumerURL)

	_, err = eventConsumerURL(request, "192.0.2.10:32000")
	require.ErrorContains(t, err, "must match")

	hostnameRequest := []byte(`<Subscribe><ConsumerReference><Address>http://localhost:8080/events</Address></ConsumerReference></Subscribe>`)
	_, err = eventConsumerURL(hostnameRequest, "127.0.0.1:32000")
	require.ErrorContains(t, err, "must be an IP address")
}

func TestEventPushRetries(t *testing.T) {
	var attempts atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < pushDeliveryTries {
			http.Error(w, "retry", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer consumer.Close()

	err := deliverEventNotifications(context.Background(), consumer.URL, []eventNotification{{
		Topic:  "tns1:VideoSource/MotionAlarm",
		Data:   `<tt:SimpleItem Value="true" Name="IsMotion"/>`,
		Source: "main",
		Time:   time.Now().UTC(),
	}})
	require.NoError(t, err)
	require.EqualValues(t, pushDeliveryTries, attempts.Load())
}

func TestParseEventDuration(t *testing.T) {
	require.Equal(t, 90*time.Second, parseEventDuration("PT1M30S", 0))
	require.Equal(t, 5*time.Second, parseEventDuration("5s", 0))
	require.Equal(t, time.Minute, parseEventDuration("invalid", time.Minute))
}

func TestEventServiceChannelQuery(t *testing.T) {
	capabilities := string(pkgonvif.GetCapabilitiesResponseWithQuery("camera.local:2984", "?channel=12"))
	require.Contains(t, capabilities, "http://camera.local:2984/onvif/event?channel=12")
	require.Contains(t, capabilities, "http://camera.local:2984/onvif/media_service?channel=12")

	services := string(pkgonvif.GetServicesResponseWithQuery("camera.local:2984", "?channel=12"))
	require.Contains(t, services, "http://camera.local:2984/onvif/event?channel=12")
}

func TestEventSubscriptionURLCompatibility(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/event?channel=3", nil)
	address := eventSubscriptionURL(request, "abc-123")
	require.Equal(t, "http://camera.local/onvif/Subscription?Idx=abc-123&channel=3", address)

	managerRequest := httptest.NewRequest(http.MethodPost, address, nil)
	require.Equal(t, "abc-123", eventSubscriptionID(managerRequest, nil))

	legacyRequest := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/event_service?subscription=legacy", nil)
	require.Equal(t, "legacy", eventSubscriptionID(legacyRequest, nil))

	lowercaseRequest := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/Subscription?idx=lowercase", nil)
	require.Equal(t, "lowercase", eventSubscriptionID(lowercaseRequest, nil))

	pathRequest := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/Subscription/path-id", nil)
	require.Equal(t, "path-id", eventSubscriptionID(pathRequest, nil))

	headerRequestBody := []byte(`<s:Header><tev:SubscriptionID>header-id</tev:SubscriptionID></s:Header>`)
	headerRequest := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/Subscription", strings.NewReader(string(headerRequestBody)))
	require.Equal(t, "header-id", eventSubscriptionID(headerRequest, headerRequestBody))
}

func TestEventMissingSubscriptionSOAPFault(t *testing.T) {
	requestBody := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:tev="http://www.onvif.org/ver10/events/wsdl"><s:Header><wsa:MessageID>urn:uuid:missing-subscription</wsa:MessageID></s:Header><s:Body><tev:PullMessages><tev:Timeout>PT0S</tev:Timeout><tev:MessageLimit>10</tev:MessageLimit></tev:PullMessages></s:Body></s:Envelope>`
	request := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/Subscription?Idx=missing", strings.NewReader(requestBody))
	recorder := httptest.NewRecorder()

	onvifDeviceService(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), `action="`+eventFaultAction+`"`)
	require.Equal(t, `"`+eventFaultAction+`"`, recorder.Header().Get("SOAPAction"))
	require.Contains(t, recorder.Body.String(), `<s:Fault`)
	require.Contains(t, recorder.Body.String(), `wsrf-r:ResourceUnknown`)
	require.Contains(t, recorder.Body.String(), `ONVIF event subscription not found`)
	require.Contains(t, recorder.Body.String(), `<wsa:RelatesTo`)
	require.Contains(t, recorder.Body.String(), `urn:uuid:missing-subscription`)
}

func TestEventAddressedResponse(t *testing.T) {
	requestBody := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://www.w3.org/2005/08/addressing"><s:Header><wsa:MessageID>urn:uuid:request-1</wsa:MessageID></s:Header><s:Body><tev:PullMessages xmlns:tev="http://www.onvif.org/ver10/events/wsdl"/></s:Body></s:Envelope>`)
	request := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/Subscription?Idx=abc-123", strings.NewReader(string(requestBody)))
	action := eventResponseAction(eventPullMessages)
	response := eventAddressedResponse(request, requestBody, eventEnvelope(`<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl"/>`), action)

	require.NoError(t, xml.Unmarshal(response, &struct{}{}))
	require.Contains(t, string(response), `<wsa:Action`)
	require.Contains(t, string(response), action)
	require.Contains(t, string(response), `<wsa:RelatesTo`)
	require.Contains(t, string(response), `urn:uuid:request-1`)
	require.Contains(t, string(response), `http://camera.local/onvif/Subscription?Idx=abc-123`)
	require.Less(t, strings.Index(string(response), `<s:Header>`), strings.Index(string(response), `<s:Body>`))
}

func TestEventHTTPResponseAction(t *testing.T) {
	requestBody := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:tev="http://www.onvif.org/ver10/events/wsdl"><s:Header><wsa:MessageID>urn:uuid:request-2</wsa:MessageID></s:Header><s:Body><tev:GetServiceCapabilities/></s:Body></s:Envelope>`
	request := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/event", strings.NewReader(requestBody))
	recorder := httptest.NewRecorder()

	onvifDeviceService(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	action := eventResponseAction(eventGetServiceCapabilities)
	require.Contains(t, recorder.Header().Get("Content-Type"), `action="`+action+`"`)
	require.Equal(t, `"`+action+`"`, recorder.Header().Get("SOAPAction"))
	require.Contains(t, recorder.Body.String(), `<wsa:Action`)
	require.Contains(t, recorder.Body.String(), `<wsa:RelatesTo`)
}
