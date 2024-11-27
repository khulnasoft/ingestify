package main

import (
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/khulnasoft/ingestify/data"
	"github.com/khulnasoft/ingestify/ingestors"
	"github.com/khulnasoft/ingestify/publishers"
	"github.com/khulnasoft/ingestify/redis"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus/hooks/writer"

	logrus_bugsnag "github.com/Shopify/logrus-bugsnag"
	bugsnag "github.com/bugsnag/bugsnag-go"
	log "github.com/sirupsen/logrus"
)

const defaultTTL = 24 * time.Hour

type Ingestify struct {
	// Place onto which jobs are placed for Libraries.io to further examine a package manager's package
	pipeline           *publishers.Pipeline
	signalHandler      chan os.Signal
	streamingIngestors []*ingestors.StreamingIngestor
}

func waitForExitSignal(signalHandler chan os.Signal) os.Signal {
	signal.Notify(signalHandler, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signalHandler
	signal.Stop(signalHandler)

	return sig
}

func main() {
	defer func() {
		// This defer will run when SIGINT is caught, but not for SIGKILL/SIGTERM/SIGHUP/SIGSTOP or os.Exit().
		log.Info("Stopping Ingestify")
	}()

	setupLogger()
	redis.Connect()

	log.Info("Starting Ingestify")
	ingestify := &Ingestify{
		pipeline:      createPipeline(),
		signalHandler: make(chan os.Signal, 1),
	}
	ingestify.registerIngestors()

	sig := waitForExitSignal(ingestify.signalHandler)

	log.WithFields(log.Fields{"signal": sig}).Info("Exiting")
}

func createPipeline() *publishers.Pipeline {
	pipeline := publishers.NewPipeline()
	pipeline.Register(&publishers.LoggingPublisher{})
	pipeline.Register(publishers.NewSidekiq())
	return pipeline
}

func (ingestify *Ingestify) registerIngestors() {
	ingestify.registerIngestor(ingestors.NewCocoaPods())
	ingestify.registerIngestor(ingestors.NewRubyGems())
	ingestify.registerIngestor(ingestors.NewElm())
	ingestify.registerIngestor(ingestors.NewGo())
	ingestify.registerIngestor(ingestors.NewMaven(ingestors.MavenCentral))
	ingestify.registerIngestor(ingestors.NewMaven(ingestors.GoogleMaven))
	ingestify.registerIngestor(ingestors.NewCargo())
	ingestify.registerIngestor(ingestors.NewNuget())
	ingestify.registerIngestor(ingestors.NewPackagist())
	ingestify.registerIngestor(ingestors.NewCPAN())
	ingestify.registerIngestor(ingestors.NewHex())
	ingestify.registerIngestor(ingestors.NewHackage())
	ingestify.registerIngestor(ingestors.NewPub())
	// drupal is returning a 403 as of 2024-05-06. need to investigate but let's
	// not block other ingestion as we do
	//	ingestify.registerIngestor(ingestors.NewDrupal())
	ingestify.registerIngestor(ingestors.NewPyPiRss())
	ingestify.registerIngestor(ingestors.NewPyPiXmlRpc())
	ingestify.registerIngestor(ingestors.NewConda(ingestors.CondaForge))
	ingestify.registerIngestor(ingestors.NewConda(ingestors.CondaMain))

	ingestify.registerIngestorStream(ingestors.NewNPM())
}

func (ingestify *Ingestify) registerIngestor(ingestor ingestors.Ingestor) {
	c := cron.New()
	ingestAndPublish := func() {
		ttl := defaultTTL

		if ttler, ok := ingestor.(ingestors.TTLer); ok {
			ttl = ttler.TTL()
		}

		for _, packageVersion := range ingestor.Ingest() {
			ingestify.pipeline.Publish(ttl, packageVersion)
		}
	}

	_, err := c.AddFunc(ingestor.Schedule(), ingestAndPublish)
	if err != nil {
		log.Fatal(err)
	}

	c.Start()

	// For now we'll run once upon registration
	ingestAndPublish()
}

func (ingestify *Ingestify) registerIngestorStream(ingestor ingestors.StreamingIngestor) {
	ingestify.streamingIngestors = append(ingestify.streamingIngestors, &ingestor)

	// Unbuffered channel so that the StreamingIngestor will block while pulling
	// next updates until Publish() has grabbed the last one.
	packageVersions := make(chan data.PackageVersion)

	go ingestor.Ingest(packageVersions)
	go func() {
		for packageVersion := range packageVersions {
			ingestify.pipeline.Publish(defaultTTL, packageVersion)
		}
	}()
}

func setupLogger() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		ForceQuote:    true,
	})

	// Configure bugsnag
	bugsnag.Configure(bugsnag.Configuration{
		APIKey:          os.Getenv("BUGSNAG_API_KEY"),
		AppVersion:      os.Getenv("GIT_COMMIT"),
		ProjectPackages: []string{"main", "github.com/khulnasoft/ingestify"},
	})
	hook, _ := logrus_bugsnag.NewBugsnagHook()
	log.AddHook(hook)

	// Send error-y logs to stderr and info-y logs to stdout
	log.SetOutput(io.Discard)
	log.AddHook(&writer.Hook{
		Writer: os.Stderr,
		LogLevels: []log.Level{
			log.PanicLevel,
			log.FatalLevel,
			log.ErrorLevel,
			log.WarnLevel,
		},
	})
	log.AddHook(&writer.Hook{
		Writer: os.Stdout,
		LogLevels: []log.Level{
			log.InfoLevel,
			log.DebugLevel,
		},
	})

	if os.Getenv("DEBUG") == "1" {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}
