package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type config struct {
	device          string
	eventURL        string
	mode            string
	listen          string
	callback        string
	topic           string
	duration        time.Duration
	subscriptionTTL time.Duration
	pullTimeout     time.Duration
	messageLimit    int
	raw             bool
}

var statusLog = log.New(os.Stderr, "", log.LstdFlags)

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		statusLog.Printf("ERROR %v", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.device, "device", "", "ONVIF device URL (required)")
	flag.StringVar(&cfg.eventURL, "event-url", "", "override the Events service URL")
	flag.StringVar(&cfg.mode, "mode", "push", "delivery mode: push or pull")
	flag.StringVar(&cfg.listen, "listen", ":18080", "push callback listen address")
	flag.StringVar(&cfg.callback, "callback", "", "public callback URL; derived automatically when empty")
	flag.StringVar(&cfg.topic, "topic", "", "optional ONVIF ConcreteSet topic filter")
	flag.DurationVar(&cfg.duration, "duration", 0, "test duration; zero runs until interrupted")
	flag.DurationVar(&cfg.subscriptionTTL, "subscription-ttl", 10*time.Minute, "requested subscription lifetime")
	flag.DurationVar(&cfg.pullTimeout, "pull-timeout", 15*time.Second, "PullMessages timeout")
	flag.IntVar(&cfg.messageLimit, "message-limit", 100, "maximum messages per PullMessages request")
	flag.BoolVar(&cfg.raw, "raw", false, "print raw SOAP event messages to stderr")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	if strings.TrimSpace(cfg.device) == "" {
		return errors.New("-device is required")
	}
	if cfg.mode != "push" && cfg.mode != "pull" {
		return fmt.Errorf("unsupported mode %q: use push or pull", cfg.mode)
	}
	if cfg.subscriptionTTL <= 0 {
		return errors.New("-subscription-ttl must be greater than zero")
	}
	if cfg.pullTimeout <= 0 {
		return errors.New("-pull-timeout must be greater than zero")
	}
	if cfg.messageLimit <= 0 {
		return errors.New("-message-limit must be greater than zero")
	}

	client, err := newEventClient(cfg.device, cfg.eventURL)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.duration)
		defer cancel()
	}

	if err = client.discover(ctx); err != nil {
		return err
	}
	statusLog.Printf("ONVIF events service: %s", displayURL(client.eventURL))
	statusLog.Printf("Delivery mode: %s", strings.ToUpper(cfg.mode))

	receiver := &eventPrinter{mode: cfg.mode, raw: cfg.raw}
	if cfg.mode == "push" {
		return client.runPush(ctx, cfg, receiver)
	}
	return client.runPull(ctx, cfg, receiver)
}
