package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *eventClient) runPush(ctx context.Context, cfg config, receiver *eventPrinter) error {
	listener, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		return fmt.Errorf("listen for ONVIF Notify: %w", err)
	}
	defer listener.Close()

	callbackURL, err := c.callbackURL(cfg.callback, listener.Addr())
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(callbackURL.Path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "ONVIF Notify requires POST", http.StatusMethodNotAllowed)
			return
		}
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		_ = r.Body.Close()
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		count, receiveErr := receiver.receive(body)
		if receiveErr != nil {
			statusLog.Printf("WARN invalid Notify message: %v", receiveErr)
			http.Error(w, receiveErr.Error(), http.StatusBadRequest)
			return
		}
		statusLog.Printf("Notify received: messages=%d remote=%s", count, r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	statusLog.Printf("Notify callback: %s", callbackURL.String())
	sub, err := c.subscribe(ctx, callbackURL.String(), cfg.topic, cfg.subscriptionTTL)
	if err != nil {
		return err
	}
	statusLog.Printf("Push subscription created: manager=%s expires=%s", displayURL(sub.manager), formatExpiration(sub.expires))
	defer c.unsubscribeWithTimeout(sub)

	for {
		renewTimer := time.NewTimer(subscriptionRenewDelay(sub.expires, cfg.subscriptionTTL))
		select {
		case <-ctx.Done():
			if !renewTimer.Stop() {
				<-renewTimer.C
			}
			statusLog.Printf("Stopping push receiver: %v", ctx.Err())
			return nil
		case serveErr := <-serverErr:
			if !renewTimer.Stop() {
				<-renewTimer.C
			}
			if errors.Is(serveErr, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("Notify callback server: %w", serveErr)
		case <-renewTimer.C:
			if err = c.renew(ctx, sub, cfg.subscriptionTTL); err != nil {
				return fmt.Errorf("renew push subscription: %w", err)
			}
			statusLog.Printf("Push subscription renewed: expires=%s", formatExpiration(sub.expires))
		}
	}
}

func (c *eventClient) subscribe(ctx context.Context, callback, topic string, ttl time.Duration) (*subscription, error) {
	var filter string
	if strings.TrimSpace(topic) != "" {
		filter = `<wsnt:Filter><wsnt:TopicExpression Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">` +
			escapeXML(strings.TrimSpace(topic)) + `</wsnt:TopicExpression></wsnt:Filter>`
	}
	operation := `<wsnt:Subscribe xmlns:wsnt="` + wsntNamespace + `" xmlns:wsa="` + wsaNamespace + `" xmlns:tns1="http://www.onvif.org/ver10/topics">` +
		`<wsnt:ConsumerReference><wsa:Address>` + escapeXML(callback) + `</wsa:Address></wsnt:ConsumerReference>` +
		filter + `<wsnt:InitialTerminationTime>` + xmlDuration(ttl) + `</wsnt:InitialTerminationTime></wsnt:Subscribe>`
	body, err := c.soap(ctx, c.eventURL, actionSubscribe, operation, nil)
	if err != nil {
		return nil, fmt.Errorf("Subscribe: %w", err)
	}
	return c.parseSubscription(body)
}

func (c *eventClient) callbackURL(raw string, listenerAddr net.Addr) (*url.URL, error) {
	if raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, errors.New("-callback must be an absolute http or https URL")
		}
		if u.Scheme != "http" {
			return nil, errors.New("-callback must use http; TLS callback serving is not configured")
		}
		if u.Path == "" {
			u.Path = "/onvif/notify"
		}
		return u, nil
	}

	host, port, err := net.SplitHostPort(listenerAddr.String())
	if err != nil {
		return nil, err
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host, err = outboundIP(c.eventURL.Hostname())
		if err != nil {
			return nil, fmt.Errorf("derive callback IP: %w; specify -callback explicitly", err)
		}
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/onvif/notify"}, nil
}

func outboundIP(remoteHost string) (string, error) {
	remote, err := net.ResolveIPAddr("ip", remoteHost)
	if err != nil {
		return "", err
	}
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: remote.IP, Port: 9})
	if err != nil {
		return "", err
	}
	defer conn.Close()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP == nil || local.IP.IsUnspecified() {
		return "", errors.New("no routable local IP found")
	}
	return local.IP.String(), nil
}
