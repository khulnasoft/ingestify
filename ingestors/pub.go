package ingestors

import (
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/khulnasoft/ingestify/data"
)

const pubSchedule = "*/5 * * * *"
const pubReleasesUrl = "https://pub.dartlang.org/feed.atom"

type pub struct {
	LatestRun time.Time
}

func NewPub() *pub {
	return &pub{}
}

func (ingestor *pub) Name() string {
	return "pub"
}

func (ingestor *pub) Schedule() string {
	return pubSchedule
}

func (ingestor *pub) Ingest() []data.PackageVersion {
	packages := ingestor.ingestURL(pubReleasesUrl)
	ingestor.LatestRun = time.Now()
	return packages
}

func (ingestor *pub) ingestURL(feedUrl string) []data.PackageVersion {
	var results []data.PackageVersion

	feed, err := ingestifyGetFeed(feedUrl)
	if err != nil {
		log.WithFields(log.Fields{"ingestor": ingestor.Name()}).Error(err)
		return results
	}

	for _, item := range feed.Items {
		// version of name is the title, for example v0.0.2 of foobar_flutter
		nameAndVersion := strings.SplitN(item.Title, " ", 3)
		results = append(results,
			data.PackageVersion{
				Platform:     ingestor.Name(),
				Name:         nameAndVersion[2],
				Version:      strings.TrimLeft(nameAndVersion[0], "v"),
				CreatedAt:    *item.UpdatedParsed,
				DiscoveryLag: time.Since(*item.UpdatedParsed),
			})
	}

	return results
}
